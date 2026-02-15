package ecogold

import (
	"log"
	"strings"
	"time"

	"arbi/internal/config"
	"arbi/internal/metrics"
	"arbi/internal/orderbook"
)

const balancePollInterval = 10 * time.Second

// StartBalanceFetcher starts a goroutine that:
// 1. Every 10 minutes: POSTs password to verify-password (keeps token valid)
// 2. Fetches balances from GET /api/balances
// Only runs when ECOGOLD_TOKEN and ECOGOLD_PASSWORD are set.
func StartBalanceFetcher() {
	if config.ECOGOLD_TOKEN == "" || config.ECOGOLD_PASSWORD == "" {
		log.Println("[EcoGold] ECOGOLD_TOKEN or ECOGOLD_PASSWORD not set, skipping balance fetcher")
		return
	}

	client := NewAuthClient(config.ECOGOLD_TOKEN, config.ECOGOLD_PASSWORD)
	exchangeName := orderbook.PriceSourceEcoGold.String()

	go func() {
		time.Sleep(5 * time.Second)

		for {
			if err := client.VerifyPassword(); err != nil {
				log.Printf("[EcoGold] Verify password failed: %v", err)
				time.Sleep(balancePollInterval)
				continue
			}

			balancesResp, err := client.GetBalances()
			if err != nil {
				log.Printf("[EcoGold] Balance fetch failed: %v", err)
				time.Sleep(balancePollInterval)
				continue
			}

			balances := make(map[string]float64)
			for _, b := range balancesResp.Data {
				symbol := strings.ToUpper(b.CurrencySymbol)
				if symbol == "" {
					continue
				}
				balances[symbol] = ParseBalance(b.Value)
			}

			metrics.UpdateWalletBalanceMetrics(exchangeName, balances)
			log.Printf("[EcoGold] Balance updated: %d wallets", len(balances))

			time.Sleep(balancePollInterval)
		}
	}()
}
