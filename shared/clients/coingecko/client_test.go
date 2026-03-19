package coingecko_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/cosmos/ibc-relayer/mocks/shared/clients/utils"
	"github.com/cosmos/ibc-relayer/shared/clients/coingecko"
)

type CoingeckoClientTestSuite struct {
	suite.Suite
	httpClient  *utils.MockHTTPClient
	priceClient *coingecko.CoingeckoClient
	baseURL     string
}

func (s *CoingeckoClientTestSuite) SetupTest() {
	s.baseURL = "https://test.coingecko.com/api/"
	s.httpClient = utils.NewMockHTTPClient(s.T())
	s.priceClient = coingecko.NewCoingeckoClient(s.httpClient, "https://", "")
}

func (s *CoingeckoClientTestSuite) TestGetSimplePrice() {
	// Given
	ctx := context.Background()
	res := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("{\"cosmos\": {\"usd\": 10.0}}")),
	}
	s.httpClient.On("Do", mock.Anything).Once().Return(res, nil).Run(func(args mock.Arguments) {
		req := args.Get(0).(*http.Request)
		s.Require().Equal("GET", req.Method)
		s.Require().Equal("/simple/price", req.URL.Path)
		s.Require().Equal("cosmos", req.URL.Query().Get("ids"))
		s.Require().Equal("usd", req.URL.Query().Get("vs_currencies"))
		s.Require().Equal(ctx, req.Context())
	})

	// When
	price, err := s.priceClient.GetSimplePrice(ctx, "cosmos", "usd")

	// Then
	s.Require().NoError(err)
	s.Require().Equal(float64(10), price)
}

func (s *CoingeckoClientTestSuite) TestGetSimplePrice_HTTPClientError() {
	// Given
	ctx := context.Background()
	s.httpClient.On("Do", mock.Anything).Once().Return(nil, errors.New("failed"))

	// When
	price, err := s.priceClient.GetSimplePrice(ctx, "cosmos", "usd")

	// Then
	s.Require().ErrorContains(err, "failed")
	s.Require().Equal(float64(0), price)
}

func (s *CoingeckoClientTestSuite) TestGetSimplePrice_ErrorCoingecko500Response() {
	// Given
	ctx := context.Background()
	res := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString("{\"cosmos\": {\"usd\": 10.0}}")),
	}
	s.httpClient.On("Do", mock.Anything).Once().Return(res, nil)

	// When
	price, err := s.priceClient.GetSimplePrice(ctx, "cosmos", "usd")

	// Then
	s.Require().ErrorContains(err, "error requesting resource from server")
	s.Require().Equal(float64(0), price)
}

func (s *CoingeckoClientTestSuite) TestGetSimplePrice_ErrorMissingPrice() {
	// Given
	ctx := context.Background()
	res := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("{\"cosmos_xyz\": {\"usd\": 10.0}}")),
	}
	s.httpClient.On("Do", mock.Anything).Once().Return(res, nil)

	// When
	price, err := s.priceClient.GetSimplePrice(ctx, "cosmos", "usd")

	// Then
	s.Require().ErrorContains(err, "failed to return price for coingeckoId")
	s.Require().Equal(float64(0), price)
}

func TestCoingeckoClientTestSuite(t *testing.T) {
	suite.Run(t, new(CoingeckoClientTestSuite))
}
