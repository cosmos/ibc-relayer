package utils

import "net/http"

type BasicAuthTransport struct {
	basicAuth *string
	transport http.RoundTripper
}

func NewBasicAuthTransport(basicAuth *string, transport http.RoundTripper) *BasicAuthTransport {
	return &BasicAuthTransport{basicAuth: basicAuth, transport: transport}
}

func (t *BasicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.basicAuth != nil {
		req.Header.Add("Authorization", "Basic "+*t.basicAuth)
	}
	if t.transport != nil {
		return t.transport.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}
