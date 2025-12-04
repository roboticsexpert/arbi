package main

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"strconv"

	"arbi/internal/arbitrage"
	"arbi/internal/config"
	"arbi/internal/metrics"
	"arbi/internal/orderbook"
	"arbi/internal/price-sources/binance"
	"arbi/internal/price-sources/ecogold"
	"arbi/internal/price-sources/kucoin"
	"arbi/internal/price-sources/nobitex"

	_ "arbi/docs" // swagger docs

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Global orderbook store - accessible from anywhere
var OrderBookStore *orderbook.Store

// Global arbitrage finder
var ArbFinder *arbitrage.Finder

// @title Arbi API
// @version 1.0
// @description Cryptocurrency Orderbook Aggregator API - Real-time orderbook data from multiple exchanges

// @host localhost:8080
// @BasePath /

// @schemes http https

func main() {
	// Initialize orderbook store
	OrderBookStore = orderbook.NewStore()

	// Get trading pairs from config
	kucoinPairs := getKucoinPairs()
	binancePairs := getBinancePairs()

	// Setup KuCoin source - automatically connects and streams orderbook data
	kucoinClient := kucoin.NewClient()
	kucoinSource := kucoin.NewSource(kucoinClient, kucoinPairs)

	// Setup Binance source - automatically connects and streams orderbook data
	binanceClient := binance.NewClient()
	binanceSource := binance.NewSource(binanceClient, binancePairs)

	// Add sources to store
	OrderBookStore.AddSource(kucoinSource)
	OrderBookStore.AddSource(binanceSource)

	// Setup EcoGold source - polls OTC prices every 30 seconds
	ecogoldSource := ecogold.NewSource()
	OrderBookStore.AddSource(ecogoldSource)

	// Setup Nobitex source - WebSocket orderbook for USDT/IRT
	nobitexSource := nobitex.NewSource([]orderbook.TradingPair{
		{Base: "USDT", Quote: "IRT"},
	})
	OrderBookStore.AddSource(nobitexSource)

	OrderBookStore.OnUpdate(func(key orderbook.OrderBookKey, ob *orderbook.OrderBook) {
		// log.Printf("[%s] %s - Best Bid: %s, Best Ask: %s",
		// 	ob.Source, ob.Pair(),
		// 	formatPrice(ob.BestBid()),
		// 	formatPrice(ob.BestAsk()))
		updateMetrics(ob)
	})

	// Start arbitrage finder - runs every 10 seconds with 100 million IRT
	ArbFinder = arbitrage.NewFinder(OrderBookStore, 100_000_000)
	go ArbFinder.Start(10 * time.Second)

	// Setup Gin router
	router := gin.Default()

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	router.GET("/up", healthCheck)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	router.GET("/orderbooks", getAllOrderbooks)
	router.GET("/orderbooks/:source", getOrderbooksBySource)
	router.GET("/orderbooks/:source/:base/:quote", getOrderbook)
	router.GET("/stats", getStats)
	router.GET("/arbitrage", getArbitrageChains)

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down...")
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

// getKucoinPairs parses KUCOIN_DEFAULT_SYMBOLS env var (format: "BTC-USDT,ETH-USDT")
func getKucoinPairs() []orderbook.TradingPair {
	symbolsStr := config.KUCOIN_DEFAULT_SYMBOLS
	if symbolsStr == "" {
		return nil
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
		ob.UpdatedAt.UnixMilli(),
	)
}
