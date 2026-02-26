package signing

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

type ConsoleSigner struct {
	Signer
}

func NewConsoleSigner(signer Signer) *ConsoleSigner {
	return &ConsoleSigner{
		Signer: signer,
	}
}

func (s *ConsoleSigner) Sign(ctx context.Context, chainID string, tx Transaction) (Transaction, error) {
	txJSON, err := tx.MarshalJSON()
	if err != nil {
		return nil, err
	}

	fmt.Println()
	fmt.Println(">>> Sending Transaction <<<")
	fmt.Println("  Chain ID:", chainID)
	fmt.Println("  Tx Data:")
	fmt.Println(string(txJSON))
	fmt.Println()

	if !promptYesNo("Do you want to send this transaction?") {
		return nil, errors.New("user abandoned transaction")
	}

	return s.Signer.Sign(ctx, chainID, tx)
}

func promptYesNo(question string) bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s (y/n): ", question)

		response, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading response. Please try again.")
			continue
		}

		response = strings.TrimSpace(response)
		response = strings.ToLower(response)

		switch response {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		default:
			fmt.Println("Invalid response. Please answer 'y' or 'n'.")
		}
	}
}
