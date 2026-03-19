package utils_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	utilsmock "github.com/cosmos/ibc-relayer/mocks/shared/clients/utils"
	"github.com/cosmos/ibc-relayer/shared/clients/utils"
)

type RateLimitedHTTPClientTestSuite struct {
	suite.Suite

	mockHTTPClient *utilsmock.MockHTTPClient
	mockLimiter    *utilsmock.MockRateLimiter
	client         *utils.RateLimitedHTTPClient
}

func (s *RateLimitedHTTPClientTestSuite) SetupTest() {
	s.mockHTTPClient = utilsmock.NewMockHTTPClient(s.T())
	s.mockLimiter = utilsmock.NewMockRateLimiter(s.T())

	s.client = utils.NewRateLimitedHTTPClient(s.mockHTTPClient, s.mockLimiter)
}

func (s *RateLimitedHTTPClientTestSuite) TestDo() {
	// Given
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://testurl.com", nil)
	s.Require().NoError(err)

	s.mockLimiter.On("Wait", mock.Anything).Once().Return(nil)
	s.mockHTTPClient.On("Do", req).Once().Return(
		&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("valid response")),
		},
		nil,
	)

	// When
	res, err := s.client.Do(req)

	// Then
	s.Require().NoError(err)
	s.Require().Equal(200, res.StatusCode)

	body, err := io.ReadAll(res.Body)
	s.Require().NoError(err)
	s.Require().Equal(body, []byte("valid response"))
}

func (s *RateLimitedHTTPClientTestSuite) TestDo_LimiterReturnsError() {
	// Given
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://testurl.com", nil)
	s.Require().NoError(err)

	s.mockLimiter.On("Wait", mock.Anything).Once().Return(errors.New("failed on limiter"))

	// When
	res, err := s.client.Do(req)

	// Then
	s.Require().ErrorContains(err, "failed on limiter")
	s.Require().Nil(res)
}

func (s *RateLimitedHTTPClientTestSuite) TestDo_ClientError() {
	// Given
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://testurl.com", nil)
	s.Require().NoError(err)

	s.mockLimiter.On("Wait", mock.Anything).Once().Return(nil)
	s.mockHTTPClient.On("Do", req).Once().Return(nil, errors.New("failed on client"))

	// When
	res, err := s.client.Do(req)

	// Then
	s.Require().ErrorContains(err, "failed on client")
	s.Require().Nil(res)
}

func (s *RateLimitedHTTPClientTestSuite) TestDo_429Response() {
	// Given
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://testurl.com", nil)
	s.Require().NoError(err)

	s.mockLimiter.On("Wait", mock.Anything).Once().Return(nil)
	s.mockHTTPClient.On("Do", req).Once().Return(
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				textproto.CanonicalMIMEHeaderKey("retry-after"): []string{"0"},
			},
		},
		nil,
	)
	s.mockHTTPClient.On("Do", req).Once().Return(
		&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("valid response")),
		},
		nil,
	)

	// When
	res, err := s.client.Do(req)

	// Then
	s.Require().NoError(err)
	s.Require().Equal(200, res.StatusCode)

	body, err := io.ReadAll(res.Body)
	s.Require().NoError(err)
	s.Require().Equal(body, []byte("valid response"))
}

func (s *RateLimitedHTTPClientTestSuite) TestDo_429Response_MissingRetryAfter() {
	// Given
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://testurl.com", nil)
	s.Require().NoError(err)

	s.mockLimiter.On("Wait", mock.Anything).Once().Return(nil)
	s.mockHTTPClient.On("Do", req).Once().Return(
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
		},
		nil,
	)

	// When
	res, err := s.client.Do(req)

	// Then
	s.Require().NoError(err)
	s.Require().Equal(429, res.StatusCode)
}

func (s *RateLimitedHTTPClientTestSuite) TestDo_429Response_InvalidRetryAfter() {
	// Given
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://testurl.com", nil)
	s.Require().NoError(err)

	s.mockLimiter.On("Wait", mock.Anything).Once().Return(nil)
	s.mockHTTPClient.On("Do", req).Once().Return(
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				textproto.CanonicalMIMEHeaderKey("retry-after"): []string{"asdf"},
			},
		},
		nil,
	)

	// When
	res, err := s.client.Do(req)

	// Then
	s.Require().NoError(err)
	s.Require().Equal(429, res.StatusCode)
}

func (s *RateLimitedHTTPClientTestSuite) TestDo_429Response_FailedAfterRetry() {
	// Given
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://testurl.com", nil)
	s.Require().NoError(err)

	s.mockLimiter.On("Wait", mock.Anything).Once().Return(nil)
	s.mockHTTPClient.On("Do", req).Once().Return(
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				textproto.CanonicalMIMEHeaderKey("retry-after"): []string{"0"},
			},
		},
		nil,
	)
	s.mockHTTPClient.On("Do", req).Once().Return(nil, errors.New("failed after retry"))

	// When
	res, err := s.client.Do(req)

	// Then
	s.Require().ErrorContains(err, "failed after retry")
	s.Require().Nil(res)
}

// Client returns Status Code 429
// returns error after retry
func TestRateLimitedHTTPClientTestSuite(t *testing.T) {
	suite.Run(t, new(RateLimitedHTTPClientTestSuite))
}
