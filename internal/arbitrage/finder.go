package arbitrage

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"arbi/internal/metrics"
	"arbi/internal/orderbook"
)

// Edge represents a possible trade or conversion in the graph
type Edge struct {
	From      string
	To        string
	Type      string // "trade" or "conversion"
	Exchange  orderbook.PriceSourceName
	Base      string
	Quote     string
	IsBuy     bool    // true = buy base with quote (use asks), false = sell base for quote (use bids)
	Rate      float64 // only for conversion type
	OrderBook *orderbook.OrderBook
}

// Finder finds arbitrage opportunities across exchanges
type Finder struct {
	store           *orderbook.Store
	conversionRates []ConversionRate
	exchangeFees    map[orderbook.PriceSourceName]float64 // fee as decimal (0.001 = 0.1%)
	startAmount     float64
	targetCurrency  string
	maxDepth        int // Maximum chain length
	lastChains      []ArbitrageChain
	mu              sync.RWMutex
	stopCh          chan struct{}
}

// NewFinder creates a new arbitrage finder
func NewFinder(store *orderbook.Store, startAmount float64) *Finder {
	return &Finder{
		store:          store,
		startAmount:    startAmount,
		targetCurrency: "IRT",
		maxDepth:       6, // Maximum 6 steps in a chain
		conversionRates: []ConversionRate{
			{From: "PAXG", To: "XAUT", Rate: 1},
			{From: "XAUT", To: "PAXG", Rate: 1},
			{From: "PAXG", To: "GOLD18", Rate: 41.4665196},
			{From: "GOLD18", To: "PAXG", Rate: 1.0 / 41.4665196},
			{From: "XAUT", To: "GOLD18", Rate: 41.4665196},
			{From: "GOLD18", To: "XAUT", Rate: 1.0 / 41.4665196},
		},
		exchangeFees: map[orderbook.PriceSourceName]float64{
			orderbook.PriceSourceBinance: 0.001, // 0.1%
			orderbook.PriceSourceKucoin:  0.001, // 0.1%
			orderbook.PriceSourceNobitex: 0.002, // 0.2%
			orderbook.PriceSourceEcoGold: 0.0,   // 0%
		},
		stopCh: make(chan struct{}),
	}
}

// getFee returns the trading fee for an exchange (as decimal)
func (f *Finder) getFee(exchange orderbook.PriceSourceName) float64 {
	if fee, ok := f.exchangeFees[exchange]; ok {
		return fee
	}
	return 0.001 // default 0.1%
}

// Start starts the arbitrage finder, running every interval
func (f *Finder) Start(interval time.Duration) {
	log.Printf("[Arbitrage] Starting finder with %.0f IRT, interval: %v", f.startAmount, interval)

	// Run immediately
	f.FindAndPrint()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			f.FindAndPrint()
		case <-f.stopCh:
			log.Println("[Arbitrage] Finder stopped")
			return
		}
	}
}

// Stop stops the arbitrage finder
func (f *Finder) Stop() {
	close(f.stopCh)
}

// FindAndPrint finds all arbitrage chains and prints them
func (f *Finder) FindAndPrint() {
	chains := f.FindAllChains()

	// Sort by profit percentage (descending)
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].ProfitPercent > chains[j].ProfitPercent
	})

	// Store the chains
	f.mu.Lock()
	f.lastChains = chains
	f.mu.Unlock()

	// Update Prometheus metrics
	metrics.ResetArbitrageMetrics()
	for _, chain := range chains {
		metrics.UpdateArbitrageMetrics(chain.Path, chain.ProfitPercent, chain.ProfitLoss)
	}

	if len(chains) == 0 {
		log.Println("[Arbitrage] No chains found")
		return
	}

	log.Printf("\n[Arbitrage] ========== Found %d chains ==========", len(chains))
	for i, chain := range chains {
		profitStr := fmt.Sprintf("%.2f%%", chain.ProfitPercent)
		if chain.ProfitLoss >= 0 {
			profitStr = "+" + profitStr
		}

		log.Printf("[%d] %s", i+1, chain.Path)
		log.Printf("    Start: %s IRT -> End: %s IRT | Profit: %s (%s IRT)",
			FormatNumber(chain.StartAmount),
			FormatNumber(chain.EndAmount),
			profitStr,
			FormatNumber(chain.ProfitLoss))
	}
	log.Println("[Arbitrage] ==========================================")
}

// GetLastChains returns the last calculated chains
func (f *Finder) GetLastChains() []ArbitrageChain {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastChains
}

// buildGraph builds a directed graph of all possible trades and conversions
func (f *Finder) buildGraph() map[string][]Edge {
	graph := make(map[string][]Edge)

	// Get all available orderbooks
	allObs := f.store.GetAll()

	// Add edges for each orderbook (both directions)
	for key, ob := range allObs {
		if ob == nil {
			continue
		}

		// Edge 1: Buy base with quote (spend quote, get base)
		// Example: BTC-USDT orderbook -> can buy BTC with USDT (use asks)
		if len(ob.Asks) > 0 {
			edge := Edge{
				From:      ob.Quote,
				To:        ob.Base,
				Type:      "trade",
				Exchange:  key.Source,
				Base:      ob.Base,
				Quote:     ob.Quote,
				IsBuy:     true,
				OrderBook: ob,
			}
			graph[ob.Quote] = append(graph[ob.Quote], edge)
		}

		// Edge 2: Sell base for quote (spend base, get quote)
		// Example: BTC-USDT orderbook -> can sell BTC for USDT (use bids)
		if len(ob.Bids) > 0 {
			edge := Edge{
				From:      ob.Base,
				To:        ob.Quote,
				Type:      "trade",
				Exchange:  key.Source,
				Base:      ob.Base,
				Quote:     ob.Quote,
				IsBuy:     false,
				OrderBook: ob,
			}
			graph[ob.Base] = append(graph[ob.Base], edge)
		}
	}

	// Add fixed conversion rate edges
	for _, conv := range f.conversionRates {
		edge := Edge{
			From: conv.From,
			To:   conv.To,
			Type: "conversion",
			Rate: conv.Rate,
		}
		graph[conv.From] = append(graph[conv.From], edge)
	}

	return graph
}

// FindAllChains finds all arbitrage cycles starting and ending with IRT (no internal loops)
func (f *Finder) FindAllChains() []ArbitrageChain {
	graph := f.buildGraph()

	// Log available edges for debugging
	log.Printf("[Arbitrage] Graph has %d currencies", len(graph))
	for currency, edges := range graph {
		var edgeStrs []string
		for _, e := range edges {
			if e.Type == "trade" {
				edgeStrs = append(edgeStrs, fmt.Sprintf("%s(%s)", e.To, e.Exchange))
			} else {
				edgeStrs = append(edgeStrs, fmt.Sprintf("%s(conv)", e.To))
			}
		}
		log.Printf("[Arbitrage]   %s -> [%s]", currency, strings.Join(edgeStrs, ", "))
	}

	var allChains []ArbitrageChain

	// DFS to find all cycles starting from IRT
	var dfs func(current string, path []Edge, visitedCurrencies map[string]bool)
	dfs = func(current string, path []Edge, visitedCurrencies map[string]bool) {
		// If we're back at IRT and have made at least 2 steps
		if current == f.targetCurrency && len(path) >= 2 {
			// Skip trivial 2-step chains on the same exchange (just buy and sell)
			// e.g., IRT-nobitex-USDT-nobitex-IRT
			if isTrivialChain(path) {
				return
			}

			chain := f.buildChainFromPath(path)
			if chain != nil {
				allChains = append(allChains, *chain)
			}
			return
		}

		// Don't exceed max depth
		if len(path) >= f.maxDepth {
			return
		}

		// Explore all edges from current currency
		for _, edge := range graph[current] {
			// Don't visit the same currency twice (except returning to IRT)
			// This prevents internal loops like: IRT->A->B->A->IRT
			if edge.To != f.targetCurrency && visitedCurrencies[edge.To] {
				continue
			}

			// Mark currency as visited and recurse
			newVisited := make(map[string]bool)
			for k, v := range visitedCurrencies {
				newVisited[k] = v
			}
			newVisited[edge.To] = true

			newPath := make([]Edge, len(path))
			copy(newPath, path)
			newPath = append(newPath, edge)

			dfs(edge.To, newPath, newVisited)
		}
	}

	// Start DFS from IRT only
	visited := map[string]bool{f.targetCurrency: true}
	dfs(f.targetCurrency, []Edge{}, visited)

	return allChains
}

// isTrivialChain checks if a path is a trivial 2-step buy/sell on the same exchange
// e.g., IRT -> USDT (nobitex) -> IRT (nobitex) is trivial
func isTrivialChain(path []Edge) bool {
	if len(path) != 2 {
		return false
	}

	// Both must be trades (not conversions)
	if path[0].Type != "trade" || path[1].Type != "trade" {
		return false
	}

	// Same exchange = trivial (just buying and selling on same market)
	return path[0].Exchange == path[1].Exchange
}

// buildChainFromPath builds an ArbitrageChain from a path of edges starting from IRT
func (f *Finder) buildChainFromPath(edges []Edge) *ArbitrageChain {
	if len(edges) == 0 {
		return nil
	}

	steps := make([]ChainStep, 0, len(edges))
	pathParts := []string{f.targetCurrency}
	currentAmount := f.startAmount

	for _, edge := range edges {
		if edge.Type == "conversion" {
			outputAmount := currentAmount * edge.Rate
			steps = append(steps, ChainStep{
				Type: "conversion",
				FixedConversion: &FixedConversionStep{
					From:      edge.From,
					To:        edge.To,
					Rate:      edge.Rate,
					AmountIn:  currentAmount,
					AmountOut: outputAmount,
				},
			})
			pathParts = append(pathParts, "convert", edge.To)
			currentAmount = outputAmount
		} else {
			var action string
			if edge.IsBuy {
				action = "buy"
			} else {
				action = "sell"
			}

			outputAmount, effectivePrice := CalculateTradeOutput(edge.OrderBook, action, currentAmount)
			if outputAmount <= 0 {
				return nil
			}

			fee := f.getFee(edge.Exchange)
			outputAmountAfterFee := outputAmount * (1 - fee)

			step := &TradeStep{
				Exchange:   edge.Exchange,
				Action:     action,
				Base:       edge.Base,
				Quote:      edge.Quote,
				Price:      effectivePrice,
				FeePercent: fee * 100,
				AmountOut:  outputAmountAfterFee,
			}

			if edge.IsBuy {
				step.Volume = currentAmount
				step.Amount = outputAmountAfterFee
			} else {
				step.Amount = currentAmount
				step.Volume = outputAmountAfterFee
			}

			steps = append(steps, ChainStep{
				Type:  "trade",
				Trade: step,
			})
			pathParts = append(pathParts, string(edge.Exchange), edge.To)
			currentAmount = outputAmountAfterFee
		}
	}

	profitLoss := currentAmount - f.startAmount
	profitPercent := (profitLoss / f.startAmount) * 100

	return &ArbitrageChain{
		Steps:         steps,
		StartCurrency: f.targetCurrency,
		EndCurrency:   f.targetCurrency,
		StartAmount:   f.startAmount,
		EndAmount:     currentAmount,
		ProfitLoss:    profitLoss,
		ProfitPercent: profitPercent,
		Path:          strings.Join(pathParts, "-"),
	}
}
