// Package jirapointssync pushes finalized poker story points back to Jira.
//
// We bypass the SDKs and call the Jira REST API directly because:
//   - The same `PUT /rest/api/2/issue/{key}` endpoint works on both Cloud and
//     Server/Data Center, so a single code path covers both.
//   - The story-points field is a customfield whose ID varies per Jira
//     deployment; passing it as a string keeps callers in control.
//
// This is intentionally fail-open: callers should log errors and continue.
package jirapointssync

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/StevenWeathers/thunderdome-planning-poker/thunderdome"
)

// jiraKeyRe matches a Jira-style issue key, e.g. "MFD-6571".
var jiraKeyRe = regexp.MustCompile(`[A-Z][A-Z0-9_]+-\d+`)

// ResolveIssueKey returns a Jira issue key from a story's reference id and/or
// link, or an empty string if no plausible key could be found.
func ResolveIssueKey(referenceID, link string) string {
	if k := strings.TrimSpace(referenceID); k != "" {
		if jiraKeyRe.MatchString(strings.ToUpper(k)) {
			return strings.ToUpper(k)
		}
	}
	if link != "" {
		if m := jiraKeyRe.FindString(strings.ToUpper(link)); m != "" {
			return m
		}
	}
	return ""
}

// ParsePoints converts a poker point string ("1", "2", "13", "1.5") to a
// float. Returns ok=false for non-numeric symbolic votes ("?", "☕️", "").
func ParsePoints(points string) (float64, bool) {
	v := strings.TrimSpace(points)
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// httpClient is a small package-local client with a sane timeout. Callers can
// override via WithHTTPClient.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// TLSOptions controls how the client trusts the Jira host's certificate.
//   - InsecureSkipVerify: disables TLS verification entirely. Only suitable for
//     internal/corporate Jira where the CA is not in the system trust store and
//     a CA bundle is unavailable.
//   - CABundlePath: path to a PEM file with additional trusted root CAs. Loaded
//     in addition to the system roots.
type TLSOptions struct {
	InsecureSkipVerify bool
	CABundlePath       string
}

var (
	customClientMu   sync.RWMutex
	customHTTPClient *http.Client
)

// ConfigureTLS rebuilds the package HTTP client according to opts. Subsequent
// UpdateIssuePoints calls use the new client. Safe to call once at startup.
func ConfigureTLS(opts TLSOptions) error {
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: opts.InsecureSkipVerify, // #nosec G402 - opt-in via config
	}
	if opts.CABundlePath != "" {
		pem, err := os.ReadFile(opts.CABundlePath)
		if err != nil {
			return fmt.Errorf("jirapointssync: read CA bundle %q: %w", opts.CABundlePath, err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return errors.New("jirapointssync: CA bundle contained no usable certificates")
		}
		tlsCfg.RootCAs = pool
	}
	customClientMu.Lock()
	customHTTPClient = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}
	customClientMu.Unlock()
	return nil
}

func activeClient() *http.Client {
	customClientMu.RLock()
	defer customClientMu.RUnlock()
	if customHTTPClient != nil {
		return customHTTPClient
	}
	return httpClient
}

// UpdateIssuePoints sets the story-points custom field on a Jira issue.
//
// `instance` provides host + auth.  Auth is selected based on
// instance.JiraDataCenter:
//   - true  -> Bearer <accessToken> (DC/Server PAT)
//   - false -> Basic base64(clientMail:accessToken) (Cloud API token)
//
// `fieldKey` is the customfield id, e.g. "customfield_11204".
func UpdateIssuePoints(ctx context.Context, instance thunderdome.JiraInstance, issueKey string, points float64, fieldKey string) error {
	if issueKey == "" {
		return fmt.Errorf("jirapointssync: empty issue key")
	}
	if fieldKey == "" {
		return fmt.Errorf("jirapointssync: empty field key")
	}
	host := strings.TrimRight(strings.TrimSpace(instance.Host), "/")
	if host == "" {
		return fmt.Errorf("jirapointssync: jira instance has no host")
	}
	if _, err := url.Parse(host); err != nil {
		return fmt.Errorf("jirapointssync: invalid host %q: %w", host, err)
	}

	endpoint := fmt.Sprintf("%s/rest/api/2/issue/%s", host, url.PathEscape(issueKey))
	body := map[string]any{
		"fields": map[string]any{
			fieldKey: points,
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("jirapointssync: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("jirapointssync: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if instance.JiraDataCenter {
		req.Header.Set("Authorization", "Bearer "+instance.AccessToken)
	} else {
		creds := instance.ClientMail + ":" + instance.AccessToken
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(creds)))
	}

	resp, err := activeClient().Do(req)
	if err != nil {
		return fmt.Errorf("jirapointssync: do: %w", err)
	}
	defer resp.Body.Close()

	// 204 No Content is the documented success response from Jira on issue update.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("jirapointssync: jira returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
}
