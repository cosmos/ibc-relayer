package http

import (
	gohttp "net/http"
)

type Client interface {
	Do(req *gohttp.Request) (*gohttp.Response, error)
}

type AuthenticatedTransport struct {
	UnderlyingTransport gohttp.RoundTripper
	Username            string
	Password            string
}

func (at *AuthenticatedTransport) RoundTrip(req *gohttp.Request) (*gohttp.Response, error) {
	req.SetBasicAuth(at.Username, at.Password)

	return at.UnderlyingTransport.RoundTrip(req)
}

func DefaultAuthenticatedTransport(username string, password string) *AuthenticatedTransport {
	return &AuthenticatedTransport{
		UnderlyingTransport: gohttp.DefaultTransport,
		Username:            username,
		Password:            password,
	}
}

func DefaultAuthenticatedClient(username string, passowrd string) *gohttp.Client {
	return &gohttp.Client{
		Transport: DefaultAuthenticatedTransport(username, passowrd),
	}
}
