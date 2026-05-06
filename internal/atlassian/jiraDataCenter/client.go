package jiradatacenter

import (
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	jira "github.com/andygrunwald/go-jira/v2/onpremise"
)

// TLSOptions controls how the SDK trusts the Jira host's certificate.
type TLSOptions struct {
	InsecureSkipVerify bool
}

var (
	tlsMu      sync.RWMutex
	tlsOptions TLSOptions
)

// ConfigureTLS applies TLS settings used by every subsequent New() call.
// Safe to call once at startup (mirrors jirapointssync.ConfigureTLS).
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
	tp := jira.BearerAuthTransport{
		Token: config.AccessToken,
		// Wrap a TLS-aware base transport so corporate / self-signed Jira
		// hosts work when CONFIG_JIRA_INSECURE_SKIP_VERIFY=true.
		Transport: &http.Transport{
			TLSClientConfig:       currentTLSConfig(),
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
	instance, err := jira.NewClient(config.InstanceHost, tp.Client())
	if err != nil {
		return nil, err
	}
	return &Client{
		instance: instance,
	}, nil
}
