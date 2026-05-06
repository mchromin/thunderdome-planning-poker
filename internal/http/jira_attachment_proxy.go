package http

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/StevenWeathers/thunderdome-planning-poker/thunderdome"
)

// jiraAttachmentClient is a dedicated HTTP client for proxying Jira-hosted
// images. It honors the same TLS overrides used by the points-sync flow so
// corporate self-signed Jiras work in both places.
var jiraAttachmentClient = func() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			// Verification flag is set lazily from config in
			// configureJiraAttachmentClient at startup.
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
}()

// configureJiraAttachmentClient applies TLS settings (used by serve.go).
func configureJiraAttachmentClient(insecureSkipVerify bool) {
	tr, ok := jiraAttachmentClient.Transport.(*http.Transport)
	if !ok || tr == nil {
		return
	}
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	tr.TLSClientConfig.InsecureSkipVerify = insecureSkipVerify // #nosec G402 - opt-in via config
}

// handleJiraAttachmentProxy fetches a Jira-hosted image (or other small asset)
// using the requesting user's stored Jira instance credentials and streams it
// back to the browser. Designed for embedding `<img>` tags from imported Jira
// descriptions which would otherwise return 401 for the unauthenticated browser.
//
// SSRF protections:
//   - Caller must be authenticated (route is wrapped with userOnly + entityUserOnly).
//   - The supplied URL's host MUST exactly match the host of one of the user's
//     own Jira instances. Any mismatch -> 400.
//   - We never follow redirects to a different host.
//
//	@Summary		Proxy a Jira-hosted image using the user's stored credentials
//	@Tags			jira
//	@Param			userId	path	string	true	"the user id"
//	@Param			url		query	string	true	"absolute Jira URL of the asset"
//	@Success		200
//	@Failure		400		object	standardJsonResponse{}
//	@Failure		502		object	standardJsonResponse{}
//	@Security		ApiKeyAuth
//	@Router			/users/{userId}/jira-attachment [get]
func (s *Service) handleJiraAttachmentProxy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sessionUserID, _ := ctx.Value(contextKeyUserID).(string)

		raw := strings.TrimSpace(r.URL.Query().Get("url"))
		if raw == "" {
			http.Error(w, "missing url", http.StatusBadRequest)
			return
		}
		target, err := url.Parse(raw)
		if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
			http.Error(w, "invalid url", http.StatusBadRequest)
			return
		}

		instances, err := s.JiraDataSvc.FindInstancesByUserID(ctx, sessionUserID)
		if err != nil {
			s.Logger.Ctx(ctx).Warn("jira attachment proxy: lookup failed", zap.Error(err))
			http.Error(w, "lookup failed", http.StatusBadGateway)
			return
		}

		var matched *thunderdome.JiraInstance
		for i := range instances {
			ih, err := url.Parse(strings.TrimRight(instances[i].Host, "/"))
			if err != nil {
				continue
			}
			if strings.EqualFold(ih.Host, target.Host) {
				matched = &instances[i]
				break
			}
		}
		if matched == nil {
			http.Error(w, "url does not match a known Jira instance", http.StatusBadRequest)
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			http.Error(w, "request build failed", http.StatusInternalServerError)
			return
		}
		if matched.JiraDataCenter {
			req.Header.Set("Authorization", "Bearer "+matched.AccessToken)
		} else {
			creds := matched.ClientMail + ":" + matched.AccessToken
			req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(creds)))
		}
		// Some Jira deployments serve different content based on Accept; ask broadly.
		req.Header.Set("Accept", "image/*,*/*;q=0.8")

		// Disallow following redirects to a different host (SSRF guard against
		// open redirect chains). Same-host redirects are still followed.
		client := *jiraAttachmentClient
		client.CheckRedirect = func(via *http.Request, prev []*http.Request) error {
			if !strings.EqualFold(via.URL.Host, target.Host) {
				return errors.New("cross-host redirect blocked")
			}
			if len(prev) >= 5 {
				return errors.New("too many redirects")
			}
			return nil
		}

		resp, err := client.Do(req)
		if err != nil {
			s.Logger.Ctx(ctx).Warn("jira attachment proxy: fetch failed",
				zap.Error(err), zap.String("url", target.String()))
			http.Error(w, "upstream fetch failed", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			http.Error(w, "upstream returned "+resp.Status, http.StatusBadGateway)
			return
		}

		// Forward useful response headers.
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		if cl := resp.Header.Get("Content-Length"); cl != "" {
			w.Header().Set("Content-Length", cl)
		}
		// Allow short browser caching to avoid repeat round-trips while a poker
		// session is open. Private because content is per-user-authorized.
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.WriteHeader(http.StatusOK)
		// Cap forwarded body to 25 MiB to prevent abuse.
		_, _ = io.Copy(w, io.LimitReader(resp.Body, 25*1024*1024))
	}
}
