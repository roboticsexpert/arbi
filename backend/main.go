package main

import (
	"log"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"strconv"

	"arbi/internal/arbitrage"
	"arbi/internal/config"
	"arbi/internal/history"
	"arbi/internal/httpx"
	"arbi/internal/metrics"
	"arbi/internal/orderbook"
	"arbi/internal/price-sources/ecogold"
	"arbi/internal/price-sources/kucoin"
	"arbi/internal/price-sources/mt5"
	"arbi/internal/price-sources/nobitex"

	"arbi/docs" // swagger docs

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Global orderbook store - accessible from anywhere
var OrderBookStore *orderbook.Store

// Global arbitrage finder
var ArbFinder *arbitrage.Finder

// Chain history recorder. nil when the database could not be opened - the
// dashboard keeps working, only the history endpoints report unavailable.
var History *history.Recorder

// Global MetaTrader 5 source - fed by the Expert Advisor via POST /api/mt5/ticks
var MT5Source *mt5.Source

// @title Arbi API
// @version 1.0
// @description Cryptocurrency Orderbook Aggregator API - Real-time orderbook data from multiple exchanges

// @BasePath /

// @schemes http https

func main() {
	// Initialize orderbook store
	OrderBookStore = orderbook.NewStore()

	// Get trading pairs from config
	kucoinPairs := getKucoinPairs()
	// binancePairs := getBinancePairs()

	// Setup KuCoin source - automatically connects and streams orderbook data
	kucoinClient := kucoin.NewClient()
	kucoinSource := kucoin.NewSource(kucoinClient, kucoinPairs)

	// Setup Binance source - automatically connects and streams orderbook data
	// binanceClient := binance.NewClient()
	// binanceSource := binance.NewSource(binanceClient, binancePairs)

	// Add sources to store
	OrderBookStore.AddSource(kucoinSource)
	// OrderBookStore.AddSource(binanceSource)

	// Setup EcoGold source - polls OTC prices every 30 seconds
	ecogoldSource := ecogold.NewSource()
	OrderBookStore.AddSource(ecogoldSource)

	// Setup Nobitex source - WebSocket orderbook (pairs from NOBITEX_DEFAULT_SYMBOLS)
	nobitexPairs := getNobitexPairs()
	nobitexSource := nobitex.NewSource(nobitexPairs)
	OrderBookStore.AddSource(nobitexSource)

	// Setup MetaTrader 5 source - a sink, fed by the EA running in the terminal
	MT5Source = mt5.NewSource(getMT5StaleAfter())
	OrderBookStore.AddSource(MT5Source)

	// Start Nobitex balance fetcher (polls every 1 min when NOBITEX_TOKEN is set)
	nobitex.StartBalanceFetcher()

	// Start EcoGold balance fetcher (verify password + fetch every 10 min when ECOGOLD_TOKEN is set)
	ecogold.StartBalanceFetcher()

	OrderBookStore.OnUpdate(func(key orderbook.OrderBookKey, ob *orderbook.OrderBook) {
		// log.Printf("[%s] %s - Best Bid: %s, Best Ask: %s",
		// 	ob.Source, ob.Pair(),
		// 	formatPrice(ob.BestBid()),
		// 	formatPrice(ob.BestAsk()))
		updateMetrics(ob)
	})

	// Start arbitrage finder - runs every 10 seconds with 100 million IRT
	ArbFinder = arbitrage.NewFinder(OrderBookStore, 100_000_000)
	History = openHistory()
	if History != nil {
		ArbFinder.OnChains(History.Record)
	}
	go ArbFinder.Start(10 * time.Second)

	// Setup Gin router
	router := gin.Default()
	router.Use(httpx.CORS(config.DASHBOARD_ORIGINS))

	// Open endpoints: the platform health check and the Prometheus scrape must
	// stay reachable without a token.
	router.GET("/up", healthCheck)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Swagger documentation - dynamically uses request host
	router.GET("/swagger/*any", swaggerHandler())

	// Data endpoints, gated by DASHBOARD_TOKEN when it is set.
	api := router.Group("/", httpx.RequireToken(config.DASHBOARD_TOKEN))
	api.GET("/orderbooks", getAllOrderbooks)
	api.GET("/orderbooks/:source", getOrderbooksBySource)
	api.GET("/orderbooks/:source/:base/:quote", getOrderbook)
	api.GET("/stats", getStats)
	api.GET("/arbitrage", getArbitrageChains)
	api.GET("/balances", getBalances)
	api.GET("/overview", getOverview)
	api.GET("/history/paths", getHistoryPaths)
	api.GET("/history", getHistory)

	// MetaTrader 5 tick ingest. This is the only write endpoint, so it is
	// registered only when a token is configured - an open ingest would let
	// anyone inject gold prices into the arbitrage graph.
	if config.MT5_INGEST_TOKEN != "" {
		router.POST("/api/mt5/ticks",
			httpx.RequireTokenHeader(httpx.MT5Header, config.MT5_INGEST_TOKEN),
			postMT5Ticks)
		log.Println("[MT5] Tick ingest enabled at POST /api/mt5/ticks")
	} else {
		log.Println("[MT5] MT5_INGEST_TOKEN not set - tick ingest disabled")
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down...")
		if History != nil {
			History.Close() // writes the partially filled minute
		}
		os.Exit(0)
	}()

	router.Run() // listens on 0.0.0.0:8080 by default
}

// healthCheck godoc
// @Summary Health check
// @Description Check if the service is running
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /up [get]
func healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "up",
	})
}

// getAllOrderbooks godoc
// @Summary Get all orderbooks
// @Description Returns all orderbooks from all exchanges
// @Tags Orderbooks
// @Produce json
// @Success 200 {object} map[string]orderbook.OrderBook
// @Router /orderbooks [get]
func getAllOrderbooks(c *gin.Context) {
	all := OrderBookStore.GetAll()
	result := make(map[string]interface{})
	for k, v := range all {
		result[k.String()] = v
	}
	c.JSON(200, result)
}

// getOrderbooksBySource godoc
// @Summary Get orderbooks by source
// @Description Returns all orderbooks for a specific price source
// @Tags Orderbooks
// @Produce json
// @Param source path string true "Price source name (e.g., kucoin, binance)"
// @Success 200 {object} map[string]orderbook.OrderBook
// @Router /orderbooks/{source} [get]
func getOrderbooksBySource(c *gin.Context) {
	source := orderbook.PriceSourceName(c.Param("source"))
	c.JSON(200, OrderBookStore.GetBySource(source))
}

// getOrderbook godoc
// @Summary Get specific orderbook
// @Description Returns the orderbook for a specific price source, base and quote currency
// @Tags Orderbooks
// @Produce json
// @Param source path string true "Price source name (e.g., kucoin, binance)"
// @Param base path string true "Base currency (e.g., BTC)"
// @Param quote path string true "Quote currency (e.g., USDT)"
// @Success 200 {object} orderbook.OrderBook
// @Failure 404 {object} map[string]string
// @Router /orderbooks/{source}/{base}/{quote} [get]
func getOrderbook(c *gin.Context) {
	source := orderbook.PriceSourceName(c.Param("source"))
	base := c.Param("base")
	quote := c.Param("quote")
	ob := OrderBookStore.Get(source, base, quote)
	if ob == nil {
		c.JSON(404, gin.H{"error": "orderbook not found"})
		return
	}
	c.JSON(200, ob)
}

// getStats godoc
// @Summary Get statistics
// @Description Returns statistics about the orderbook store
// @Tags Stats
// @Produce json
// @Success 200 {object} orderbook.StoreStats
// @Router /stats [get]
func getStats(c *gin.Context) {
	c.JSON(200, OrderBookStore.Stats())
}

// getArbitrageChains godoc
// @Summary Get arbitrage chains
// @Description Returns the latest calculated arbitrage opportunities
// @Tags Arbitrage
// @Produce json
// @Success 200 {array} arbitrage.ArbitrageChain
// @Router /arbitrage [get]
func getArbitrageChains(c *gin.Context) {
	chains := ArbFinder.GetLastChains()
	c.JSON(200, gin.H{
		"count":  len(chains),
		"chains": chains,
	})
}

// getBalances godoc
// @Summary Get wallet balances
// @Description Returns the latest wallet balances per price source
// @Tags Balances
// @Produce json
// @Success 200 {array} metrics.ExchangeBalances
// @Router /balances [get]
func getBalances(c *gin.Context) {
	c.JSON(200, metrics.GetBalanceSnapshots())
}

// getOverview godoc
// @Summary Dashboard overview
// @Description Returns orderbooks, arbitrage chains, balances and stats in one payload
// @Tags Dashboard
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /overview [get]
func getOverview(c *gin.Context) {
	all := OrderBookStore.GetAll()
	books := make([]*orderbook.OrderBook, 0, len(all))
	for _, ob := range all {
		books = append(books, ob)
	}
	sort.Slice(books, func(i, j int) bool {
		if books[i].Source != books[j].Source {
			return books[i].Source < books[j].Source
		}
		return books[i].Pair() < books[j].Pair()
	})

	chains := ArbFinder.GetLastChains()

	c.JSON(200, gin.H{
		"server_time": time.Now(),
		"stats":       OrderBookStore.Stats(),
		"orderbooks":  books,
		"arbitrage":   chains,
		"balances":    metrics.GetBalanceSnapshots(),
	})
}

// openHistory opens the chain history database. A failure (typically an
// unwritable volume) is logged and disables history rather than the service.
func openHistory() *history.Recorder {
	path := config.HISTORY_DB_PATH
	if path == "" {
		path = "data/history.db"
	}
	days := 30
	if v, err := strconv.Atoi(config.HISTORY_RETENTION_DAYS); err == nil && v >= 0 {
		days = v
	}
	rec, err := history.Open(path, time.Duration(days)*24*time.Hour)
	if err != nil {
		log.Printf("[History] disabled - cannot open %s: %v", path, err)
		return nil
	}
	log.Printf("[History] recording chains to %s (retention %d days)", path, days)
	return rec
}

// getHistoryPaths godoc
// @Summary Chain paths with history
// @Description Every arbitrage chain path that has recorded history, most recently seen first
// @Tags History
// @Produce json
// @Success 200 {array} history.PathInfo
// @Failure 503 {object} map[string]string
// @Router /history/paths [get]
func getHistoryPaths(c *gin.Context) {
	if History == nil {
		c.JSON(503, gin.H{"error": "history is disabled"})
		return
	}
	paths, err := History.Paths()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, paths)
}

// maxHistoryPaths bounds a single /history request.
const maxHistoryPaths = 10

// getHistory godoc
// @Summary Chain profit history
// @Description Profit percentage per chain path in minute buckets (or coarser for long ranges)
// @Tags History
// @Produce json
// @Param path query []string true "Chain path, repeatable (max 10)" collectionFormat(multi)
// @Param from query int false "Range start, unix seconds (default: 24h ago)"
// @Param to query int false "Range end, unix seconds (default: now)"
// @Param step query int false "Bucket size in minutes: 1, 5, 15, 60, 240 or 1440 (default: auto)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /history [get]
func getHistory(c *gin.Context) {
	if History == nil {
		c.JSON(503, gin.H{"error": "history is disabled"})
		return
	}

	paths := c.QueryArray("path")
	if len(paths) == 0 || len(paths) > maxHistoryPaths {
		c.JSON(400, gin.H{"error": "pass between 1 and 10 path parameters"})
		return
	}

	to := time.Now()
	if v, err := strconv.ParseInt(c.Query("to"), 10, 64); err == nil {
		to = time.Unix(v, 0)
	}
	from := to.Add(-24 * time.Hour)
	if v, err := strconv.ParseInt(c.Query("from"), 10, 64); err == nil {
		from = time.Unix(v, 0)
	}
	if !from.Before(to) {
		c.JSON(400, gin.H{"error": "from must be before to"})
		return
	}

	step := history.AutoStep(from, to)
	if v, err := strconv.Atoi(c.Query("step")); err == nil {
		if !slices.Contains(history.Steps, v) {
			c.JSON(400, gin.H{"error": "step must be one of 1, 5, 15, 60, 240, 1440"})
			return
		}
		step = v
	}

	series, err := History.Series(paths, from, to, step)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"from":   from.Unix(),
		"to":     to.Unix(),
		"step":   step,
		"series": series,
	})
}

// postMT5Ticks godoc
// @Summary Ingest MetaTrader 5 ticks
// @Description Accepts top-of-book quotes pushed by the MetaTrader 5 Expert Advisor
// @Tags MT5
// @Accept json
// @Produce json
// @Param ticks body mt5.PushRequest true "Ticks"
// @Success 200 {object} mt5.PushResult
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /api/mt5/ticks [post]
func postMT5Ticks(c *gin.Context) {
	var req mt5.PushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}
	if len(req.Ticks) == 0 {
		c.JSON(400, gin.H{"error": "no ticks in payload"})
		return
	}

	result := MT5Source.Push(req.Ticks)
	if len(result.Accepted) == 0 {
		// Every tick bounced - almost always a symbol-name mismatch, so say
		// which names would have worked instead of returning a bare 200.
		c.JSON(400, gin.H{
			"error":         "no ticks accepted",
			"skipped":       result.Skipped,
			"known_symbols": mt5.KnownSymbols(),
		})
		return
	}

	c.JSON(200, result)
}

// getMT5StaleAfter reads MT5_STALE_SECONDS, falling back to the package default
func getMT5StaleAfter() time.Duration {
	if config.MT5_STALE_SECONDS == "" {
		return mt5.DefaultStaleAfter
	}
	secs, err := strconv.Atoi(config.MT5_STALE_SECONDS)
	if err != nil || secs <= 0 {
		log.Printf("[MT5] Invalid MT5_STALE_SECONDS %q, using %v", config.MT5_STALE_SECONDS, mt5.DefaultStaleAfter)
		return mt5.DefaultStaleAfter
	}
	return time.Duration(secs) * time.Second
}

// getKucoinPairs parses KUCOIN_DEFAULT_SYMBOLS env var (format: "BTC-USDT,ETH-USDT")
func getKucoinPairs() []orderbook.TradingPair {
	symbolsStr := config.KUCOIN_DEFAULT_SYMBOLS
	if symbolsStr == "" {
		return nil
	}
	return parsePairs(strings.Split(symbolsStr, ","))
}

// getNobitexPairs parses NOBITEX_DEFAULT_SYMBOLS env var (format: "USDT-IRT,BTC-IRT")
func getNobitexPairs() []orderbook.TradingPair {
	symbolsStr := config.NOBITEX_DEFAULT_SYMBOLS
	if symbolsStr == "" {
		// Default: USDT/IRT if not configured
		return []orderbook.TradingPair{{Base: "USDT", Quote: "IRT"}}
	}
	return parsePairs(strings.Split(symbolsStr, ","))
}

// getBinancePairs parses BINANCE_DEFAULT_SYMBOLS env var (format: "BTC-USDT,ETH-USDT")
func getBinancePairs() []orderbook.TradingPair {
	symbolsStr := config.BINANCE_DEFAULT_SYMBOLS
	if symbolsStr == "" {
		return nil
	}
	return parsePairs(strings.Split(symbolsStr, ","))
}

// parsePairs converts symbol strings like "BTC-USDT" to TradingPair structs
func parsePairs(symbols []string) []orderbook.TradingPair {
	pairs := make([]orderbook.TradingPair, 0, len(symbols))
	for _, s := range symbols {
		parts := strings.Split(strings.TrimSpace(s), "-")
		if len(parts) == 2 {
			pairs = append(pairs, orderbook.TradingPair{
				Base:  parts[0],
				Quote: parts[1],
			})
		}
	}
	return pairs
}

func formatPrice(pl *orderbook.PriceLevel) string {
	if pl == nil {
		return "N/A"
	}
	return pl.Price + " @ " + pl.Quantity
}

// swaggerHandler returns a handler that dynamically sets the Swagger host based on the request
func swaggerHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get host from request, checking X-Forwarded-Host first (for reverse proxies)
		host := c.Request.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = c.Request.Host
		}
		if host == "" {
			host = c.Request.Header.Get("Host")
		}
		if host == "" {
			host = "localhost:8080" // fallback
		}

		// Dynamically update SwaggerInfo host
		docs.SwaggerInfo.Host = host

		// Use the standard swagger handler
		ginSwagger.WrapHandler(swaggerFiles.Handler)(c)
	}
}

// updateMetrics updates Prometheus metrics for an orderbook
func updateMetrics(ob *orderbook.OrderBook) {
	var bidPrice, askPrice float64

	if bestBid := ob.BestBid(); bestBid != nil {
		bidPrice, _ = strconv.ParseFloat(bestBid.Price, 64)
	}

	if bestAsk := ob.BestAsk(); bestAsk != nil {
		askPrice, _ = strconv.ParseFloat(bestAsk.Price, 64)
	}

	metrics.UpdateOrderbookMetrics(
		ob.Source.String(),
		ob.Base,
		ob.Quote,
		bidPrice,
		askPrice,
		ob.UpdatedAt.Unix(),
	)
}
