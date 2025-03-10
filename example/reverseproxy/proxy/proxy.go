package reverseproxy

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/opd-ai/paywall"
)

/*
This example demonstrates how to use the paywall package to protect a reverse proxy.
The reverse proxy is configured to forward requests to a target server, but only if the client has paid the required amount.
The paywall is configured to accept payments in Bitcoin and Monero on the respective live nets.
This is a useful service in it's own right for adding paywalls go non-Go HTTP applications.
The user may optionally set a ReverseProxy.ProtectedPath to specify the path that requires payment.
*/

// Proxy represents a reverse proxy that enforces a paywall
type Proxy struct {
	*paywall.Paywall
	*ReverseProxy
	ProtectedPath string
}

// NewProxy creates a new Proxy instance
// Parameters:
//   - target: The URL of the server to forward requests to
//   - paywall: The paywall instance to enforce
//
// Returns:
//   - *Proxy: A new reverse proxy with paywall protection
func NewProxy(target string, p *paywall.Paywall) (*Proxy, error) {
	rp, err := NewReverseProxy(target)
	if err != nil {
		return nil, err
	}
	return &Proxy{
		Paywall:      p,
		ReverseProxy: rp,
	}, nil
}

// ServeHTTP forwards incoming HTTP requests to the target server
// Parameters:
//   - w: The HTTP response writer
//   - r: The HTTP request to forward
//
// Flow:
//  1. Checks if the request path is protected
//  2. If protected, enforces paywall
//  3. Forwards the request to the target server if payment is confirmed
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p.ProtectedPath != "" && checkPath(r.URL.Path, p.ProtectedPath) {
		p.Middleware(p.ReverseProxy).ServeHTTP(w, r)
		return
	}
	if p.ProtectedPath == "" {
		p.Middleware(p.ReverseProxy).ServeHTTP(w, r)
		return
	}
	p.ReverseProxy.ServeHTTP(w, r)
}

func checkPath(path, protected string) bool {
	return strings.HasPrefix(strings.TrimLeft(path, string(filepath.Separator)), strings.TrimLeft(protected, string(filepath.Separator)))
}
