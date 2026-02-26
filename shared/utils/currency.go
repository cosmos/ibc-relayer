package utils

import (
	"math/big"
)

func FormatCoin(amount *big.Int, decimals int) *big.Float {
	decimalsBig := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	amountFloat := new(big.Float).SetInt(amount)
	amountFloat = new(big.Float).Quo(amountFloat, new(big.Float).SetInt(decimalsBig))
	return amountFloat
}
