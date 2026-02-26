package utils

import (
	"context"
	"encoding/base64"
	"net/http"
)

//go:generate mockery --name HTTPClient --filename mock_http_client.go
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

//go:generate mockery --name RateLimiter --filename mock_rate_limiter.go
type RateLimiter interface {
	Wait(ctx context.Context) error
}

func BasicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}
