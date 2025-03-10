package reverseproxy

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ReverseProxy represents a reverse proxy that forwards requests to a target server
type ReverseProxy struct {
	// Target is the URL of the server to forward requests to, must be a valid URL
	Target *url.URL
	// Transport is the HTTP transport used to make requests to the target server
	Transport http.RoundTripper
}

// NewReverseProxy creates a new ReverseProxy instance
// Parameters:
//   - target: The URL of the server to forward requests to
//
// Returns:
//   - *ReverseProxy: A new reverse proxy instance
func NewReverseProxy(target string) (*ReverseProxy, error) {
	// Validate target URL
	url, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("parse target URL: %w", err)
	}
	if url.Scheme == "" || url.Host == "" {
		return nil, fmt.Errorf("target URL must include scheme and host")
	}
	return &ReverseProxy{
		Target:    url,
		Transport: http.DefaultTransport,
	}, nil
}

// ServeHTTP forwards incoming HTTP requests to the target server
// Parameters:
//   - w: The HTTP response writer
//   - r: The HTTP request to forward
//
// Flow:
//  1. Forwards the request to the target server
//  2. Uses the ReverseProxy.Transport to make the request
//  3. Sets the request URL scheme and host to the target server
//  4. Forwards the request to the target server
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Forward request to target server
	// create a proxy request to send to the target server
	proxyRequest := http.Request{
		Method: r.Method,
		URL:    rp.Target,
		Proto:  r.Proto,
		Header: r.Header,
		Body:   r.Body,
	}
	// set the host to the target server
	proxyRequest.Host = rp.Target.Host
	// set the scheme to the target server
	proxyRequest.URL.Scheme = rp.Target.Scheme
	// send the request using the transport
	response, err := rp.Transport.RoundTrip(&proxyRequest)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy request failed: %v", err), http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	// Forward the response from the target server to the client
	for key, values := range response.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	//write the response body to the client
	_, err = io.Copy(w, response.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to write response: %v", err), http.StatusInternalServerError)
	}
}
