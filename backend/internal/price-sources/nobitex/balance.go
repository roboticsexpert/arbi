package nobitex

import (
	"log"
	"strings"
	"time"

	"arbi/internal/config"
	"arbi/internal/metrics"
	"arbi/internal/orderbook"
)

const balancePollInterval = 10 * time.Second

// normalizeNobitexBalance converts Nobitex-specific units to standard symbols.
// RLS (Rial) -> IRT (Toman): 1 IRT = 10 RLS
func normalizeNobitexBalance(currency, balanceStr string) (symbol string, balance float64) {
	balance = ParseBalance(balanceStr)
	symbol = strings.ToUpper(currency)
	if symbol == "" {
		symbol = "RLS"
	}

	switch symbol {
	case "RLS":
		// RLS (Rial) -> IRT (Toman): divide by 10
		return "IRT", balance / 10
	default:
		return symbol, balance
	}
}

// StartBalanceFetcher starts a goroutine that fetches Nobitex wallet balances
// every minute and updates Prometheus metrics. Only runs when NOBITEX_TOKEN is set.
func StartBalanceFetcher() {
	if config.NOBITEX_TOKEN == "" {
		log.Println("[Nobitex] NOBITEX_TOKEN not set, skipping balance fetcher")
		return
	}

	client := NewClient(config.NOBITEX_TOKEN)
	exchangeName := orderbook.PriceSourceNobitex.String()

	go func() {
		// Initial fetch after a short delay (let the app start first)
		time.Sleep(5 * time.Second)

		for {
			wallets, err := client.GetWallets()
			if err != nil {
				log.Printf("[Nobitex] Balance fetch failed: %v", err)
				time.Sleep(balancePollInterval)
				continue
			}

			balances := make(map[string]float64)
			for _, w := range wallets {
				currency, balance := normalizeNobitexBalance(w.Currency, w.Balance)
				balances[currency] = balance
			}

			metrics.UpdateWalletBalanceMetrics(exchangeName, balances)
			log.Printf("[Nobitex] Balance updated: %d wallets", len(balances))

			time.Sleep(balancePollInterval)
		}
	}()
}
