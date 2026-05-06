// Package jira provides JIRA cloud integration
package jira

import (
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	jira "github.com/ctreminiom/go-atlassian/v2/jira/v3"
)

// TLSOptions mirrors the DC variant so corporate self-signed Jira hosts work
// when CONFIG_JIRA_INSECURE_SKIP_VERIFY=true.
type TLSOptions struct {
	InsecureSkipVerify bool
}

var (
	tlsMu      sync.RWMutex
	tlsOptions TLSOptions
)

// ConfigureTLS applies TLS settings used by subsequent New() calls.
func ConfigureTLS(opts TLSOptions) {
	tlsMu.Lock()
	defer tlsMu.Unlock()
	tlsOptions = opts
}

func currentTLSConfig() *tls.Config {
	tlsMu.RLock()
	defer tlsMu.RUnlock()
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: tlsOptions.InsecureSkipVerify, // #nosec G402 - opt-in via config
	}
}

// New creates a new JIRA client
func New(config Config) (*Client, error) {

	httpClient := http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			TLSClientConfig:       currentTLSConfig(),
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
	instance, err := jira.New(&httpClient, config.InstanceHost)

	if err != nil {
		return nil, err
	}
	instance.Auth.SetBasicAuth(config.ClientMail, config.AccessToken)

	return &Client{
		instance: instance,
	}, nil
}
