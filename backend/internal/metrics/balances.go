package metrics

import (
	"sync"
	"time"
)

// Balance snapshots mirror what is pushed into the Prometheus wallet gauges so
// the dashboard can read them over REST without scraping /metrics.

// ExchangeBalances is the balance snapshot for a single price source.
type ExchangeBalances struct {
	Exchange    string             `json:"exchange"`
	Balances    map[string]float64 `json:"balances"`
	LastFetched time.Time          `json:"last_fetched"`
}

var (
	balanceMu    sync.RWMutex
	balanceStore = map[string]ExchangeBalances{}
)

// GetBalanceSnapshots returns a copy of the latest balances per exchange.
func GetBalanceSnapshots() []ExchangeBalances {
	balanceMu.RLock()
	defer balanceMu.RUnlock()

	out := make([]ExchangeBalances, 0, len(balanceStore))
	for _, snap := range balanceStore {
		balances := make(map[string]float64, len(snap.Balances))
		for k, v := range snap.Balances {
			balances[k] = v
		}
		snap.Balances = balances
		out = append(out, snap)
	}
	return out
}

// storeBalanceSnapshot records the balances for an exchange for REST consumers.
func storeBalanceSnapshot(exchange string, balances map[string]float64) {
	copied := make(map[string]float64, len(balances))
	for k, v := range balances {
		copied[k] = v
	}

	balanceMu.Lock()
	defer balanceMu.Unlock()
	balanceStore[exchange] = ExchangeBalances{
		Exchange:    exchange,
		Balances:    copied,
		LastFetched: time.Now(),
	}
}
