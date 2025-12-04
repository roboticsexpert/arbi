package main

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"strconv"

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

// @title Arbi API
// @version 1.0
// @description Cryptocurrency Orderbook Aggregator API - Real-time orderbook data from multiple exchanges

// @host localhost:8080
// @BasePath /

// @schemes http https

func main() {
	// Initialize orderbook store
	OrderBookStore = orderbook.NewStore()

	// Get symbols from config
	kucoinSymbols := getKucoinSymbols()
	binanceSymbols := getBinanceSymbols()

	// Setup KuCoin source - automatically connects and streams orderbook data
	kucoinClient := kucoin.NewClient()
	kucoinSource := kucoin.NewSource(kucoinClient, kucoinSymbols)

	// Setup Binance source - automatically connects and streams orderbook data
	binanceClient := binance.NewClient()
	binanceSource := binance.NewSource(binanceClient, binanceSymbols)

	// Add sources to store
	OrderBookStore.AddSource(kucoinSource)
	OrderBookStore.AddSource(binanceSource)

	// Setup EcoGold source - polls OTC prices every 30 seconds
	ecogoldSource := ecogold.NewSource()
	OrderBookStore.AddSource(ecogoldSource)

	// Setup Nobitex source - WebSocket orderbook for USDT/IRT
	nobitexSource := nobitex.NewSource([]string{"USDTIRT"})
	OrderBookStore.AddSource(nobitexSource)

	OrderBookStore.OnUpdate(func(key orderbook.OrderBookKey, ob *orderbook.OrderBook) {
		log.Printf("[%s] %s - Best Bid: %s, Best Ask: %s",
			ob.Source, ob.Symbol,
			formatPrice(ob.BestBid()),
			formatPrice(ob.BestAsk()))
	})

	// Setup Gin router
	router := gin.Default()

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	router.GET("/up", healthCheck)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	router.GET("/orderbooks", getAllOrderbooks)
	router.GET("/orderbooks/:source", getOrderbooksBySource)
	router.GET("/orderbooks/:source/:symbol", getOrderbook)
	router.GET("/stats", getStats)

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
// @Description Returns the orderbook for a specific price source and symbol
// @Tags Orderbooks
// @Produce json
// @Param source path string true "Price source name (e.g., kucoin, binance)"
// @Param symbol path string true "Trading pair symbol (e.g., BTC-USDT)"
// @Success 200 {object} orderbook.OrderBook
// @Failure 404 {object} map[string]string
// @Router /orderbooks/{source}/{symbol} [get]
func getOrderbook(c *gin.Context) {
	source := orderbook.PriceSourceName(c.Param("source"))
	symbol := c.Param("symbol")
	ob := OrderBookStore.Get(source, symbol)
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

func getKucoinSymbols() []string {
	symbolsStr := config.KUCOIN_DEFAULT_SYMBOLS
	if symbolsStr == "" {
		return nil
	}
	return strings.Split(symbolsStr, ",")
}

func getBinanceSymbols() []string {
	symbolsStr := config.BINANCE_DEFAULT_SYMBOLS
	if symbolsStr == "" {
		return nil
	}
	return strings.Split(symbolsStr, ",")
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
		ob.Symbol,
		bidPrice,
		askPrice,
		ob.UpdatedAt.UnixMilli(),
	)
}
