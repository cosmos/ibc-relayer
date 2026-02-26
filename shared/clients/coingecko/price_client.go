package coingecko

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync"
	"time"

	sdk_math "cosmossdk.io/math"
	"github.com/jackc/pgx/v5/pgtype"
)

//go:generate mockery --name PriceClient --filename mock_price_client.go
type PriceClient interface {
	GetSimplePrice(ctx context.Context, coingeckoID string, currency string) (float64, error)
}

type CachedPriceClient struct {
	mu                   sync.RWMutex
	internalClient       PriceClient
	cache                map[string]PriceResponse
	cacheRefreshInterval time.Duration
}

type PriceResponse struct {
	price       float64
	lastUpdated time.Time
}

func NewCachedPriceClient(client PriceClient, cacheRefreshInterval time.Duration) *CachedPriceClient {
	return &CachedPriceClient{
		internalClient:       client,
		cache:                make(map[string]PriceResponse),
		cacheRefreshInterval: cacheRefreshInterval,
	}
}

func cacheKey(coingeckoID string, currency string) string {
	return fmt.Sprintf("%s/%s", strings.ReplaceAll(coingeckoID, "/", "//"), strings.ReplaceAll(currency, "/", "//"))
}

func (c *CachedPriceClient) GetSimplePrice(ctx context.Context, coingeckoID string, currency string) (float64, error) {
	key := cacheKey(coingeckoID, currency)
	c.mu.RLock()
	priceResponse, ok := c.cache[key]
	c.mu.RUnlock()
	if !ok || time.Since(priceResponse.lastUpdated) > c.cacheRefreshInterval {
		var err error
		price, err := c.internalClient.GetSimplePrice(ctx, coingeckoID, currency)
		if err != nil {
			return 0, err
		}
		priceResponse = PriceResponse{
			price:       price,
			lastUpdated: time.Now(),
		}
		c.mu.Lock()
		c.cache[key] = priceResponse
		c.mu.Unlock()
	}
	return priceResponse.price, nil
}

func (c *CachedPriceClient) GetCoinUsdValue(ctx context.Context, coingeckoID string, decimals uint8, amount *big.Int) (pgtype.Numeric, error) {
	coingeckoPrice, err := c.GetSimplePrice(ctx, coingeckoID, "usd")
	if err != nil {
		return pgtype.Numeric{}, fmt.Errorf("getting coingecko price of %s in usd: %w", coingeckoID, err)
	}

	amountFloat, err := sdk_math.LegacyNewDecFromInt(sdk_math.NewIntFromBigInt(amount)).Float64()
	if err != nil {
		return pgtype.Numeric{}, fmt.Errorf("invalid amount (%s) to float64 conversion: %w", amount.String(), err)
	}
	usdAmountStr := fmt.Sprintf("%f", amountFloat/(math.Pow10(int(decimals)))*coingeckoPrice)

	num := &pgtype.Numeric{Valid: true}
	if err = num.ScanScientific(usdAmountStr); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("scanning usd amount %s into pg numeric: %w", usdAmountStr, err)
	}

	return *num, nil
}

type NoOpPriceClient struct{}

func NewNoOpPriceClient() PriceClient {
	return &NoOpPriceClient{}
}

func (m *NoOpPriceClient) GetSimplePrice(ctx context.Context, coingeckoID string, currency string) (float64, error) {
	return 0, nil
}

func (c *NoOpPriceClient) GetCoinUsdValue(ctx context.Context, coingeckoID string, decimals uint8, amount *big.Int) (pgtype.Numeric, error) {
	return pgtype.Numeric{}, nil
}
