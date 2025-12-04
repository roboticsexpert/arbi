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
			{From: "PAXG", To: "GOLD18", Rate: 41.4665196},
			{From: "GOLD18", To: "PAXG", Rate: 1.0 / 41.4665196},
		},
		stopCh: make(chan struct{}),
	}
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

// FindAllChains finds all arbitrage chains starting and ending with targetCurrency
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

	// DFS to find all paths from targetCurrency back to targetCurrency
	var dfs func(current string, path []Edge, visited map[string]bool, amount float64)
	dfs = func(current string, path []Edge, visited map[string]bool, amount float64) {
		// If we're back at target currency and have made at least one trade
		if current == f.targetCurrency && len(path) > 0 {
			chain := f.buildChainFromPath(path, amount)
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
			// For trades, we can use the same currency on different exchanges
			// So we track visited as "currency:exchange" for trades
			// For conversions, just track currency
			var visitKey string
			if edge.Type == "trade" {
				visitKey = edge.To + ":" + string(edge.Exchange)
			} else {
				visitKey = edge.To + ":conversion"
			}

			// Allow returning to target currency, but not other revisits
			if edge.To != f.targetCurrency && visited[visitKey] {
				continue
			}

			// Calculate output amount for this edge
			newAmount := f.calculateEdgeOutput(edge, amount)
			if newAmount <= 0 {
				continue
			}

			// Mark as visited and recurse
			newVisited := make(map[string]bool)
			for k, v := range visited {
				newVisited[k] = v
			}
			newVisited[visitKey] = true

			newPath := make([]Edge, len(path))
			copy(newPath, path)
			newPath = append(newPath, edge)

			dfs(edge.To, newPath, newVisited, newAmount)
		}
	}

	// Start DFS from target currency
	visited := make(map[string]bool)
	dfs(f.targetCurrency, []Edge{}, visited, f.startAmount)

	return allChains
}

// calculateEdgeOutput calculates the output amount after traversing an edge
func (f *Finder) calculateEdgeOutput(edge Edge, inputAmount float64) float64 {
	if edge.Type == "conversion" {
		return inputAmount * edge.Rate
	}

	// Trade type
	if edge.OrderBook == nil {
		return 0
	}

	var action string
	if edge.IsBuy {
		action = "buy"
	} else {
		action = "sell"
	}

	output, _ := CalculateTradeOutput(edge.OrderBook, action, inputAmount)
	return output
}

// buildChainFromPath creates an ArbitrageChain from a path of edges
func (f *Finder) buildChainFromPath(path []Edge, finalAmount float64) *ArbitrageChain {
	if len(path) == 0 {
		return nil
	}

	steps := make([]ChainStep, 0, len(path))
	pathParts := []string{f.targetCurrency}
	currentAmount := f.startAmount

	for _, edge := range path {
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
			// Format: IRT-convert-GOLD18
			pathParts = append(pathParts, "convert", edge.To)
			currentAmount = outputAmount
		} else {
			// Trade
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

			step := &TradeStep{
				Exchange:  edge.Exchange,
				Action:    action,
				Base:      edge.Base,
				Quote:     edge.Quote,
				Price:     effectivePrice,
				AmountOut: outputAmount,
			}

			if edge.IsBuy {
				step.Volume = currentAmount // We spend quote
				step.Amount = outputAmount  // We get base
			} else {
				step.Amount = currentAmount // We spend base
				step.Volume = outputAmount  // We get quote
			}

			steps = append(steps, ChainStep{
				Type:  "trade",
				Trade: step,
			})
			// Format: IRT-nobitex-USDT
			pathParts = append(pathParts, string(edge.Exchange), edge.To)
			currentAmount = outputAmount
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
