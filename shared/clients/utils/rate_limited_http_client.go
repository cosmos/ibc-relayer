package utils

import (
	"net/http"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

type RateLimitedHTTPClient struct {
	client  HTTPClient
	limiter RateLimiter
}

func NewRateLimitedHTTPClient(client HTTPClient, limiter RateLimiter) *RateLimitedHTTPClient {
	return &RateLimitedHTTPClient{client, limiter}
}

func DefaultRateLimitedHTTPClient(requestsPerMinute int) *RateLimitedHTTPClient {
	if requestsPerMinute == 0 {
		requestsPerMinute = 10
	}
	return NewRateLimitedHTTPClient(
		http.DefaultClient,
		rate.NewLimiter(rate.Every(time.Minute/time.Duration(requestsPerMinute)), requestsPerMinute/2),
	)
}

func (c *RateLimitedHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if err := c.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}

	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	// if we get rate limited anyway, check for retry-after header, sleep, and try again
	if res.StatusCode == http.StatusTooManyRequests {
		retryAfter, parseErr := strconv.Atoi(res.Header.Get("retry-after"))
		if parseErr != nil {
			return res, nil
		}

		select {
		case <-req.Context().Done():
		case <-time.After(time.Duration(retryAfter) * time.Second):
		}

		res, err = c.client.Do(req)
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}
