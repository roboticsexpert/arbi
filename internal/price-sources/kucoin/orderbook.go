package kucoin

import (
	"log"
	"strings"
	"sync"
	"time"

	"arbi/internal/orderbook"

	"github.com/Kucoin/kucoin-universal-sdk/sdk/golang/pkg/generate/spot/spotpublic"
)

const SourceName = orderbook.PriceSourceKucoin

// Source implements the orderbook.PriceSource interface for KuCoin
// It automatically connects and streams 50-level orderbook data
type Source struct {
	client       *Client
	spotPublicWs spotpublic.SpotPublicWS
	orderBooks   map[string]*orderbook.OrderBook // key: "base-quote"
	callbacks    []func(*orderbook.OrderBook)
	mu           sync.RWMutex
}

// NewSource creates a new KuCoin price source and automatically starts streaming
// orderbook data for the given pairs (50-level depth by default)
// pairs format: []orderbook.TradingPair{{Base: "BTC", Quote: "USDT"}, ...}
func NewSource(client *Client, pairs []orderbook.TradingPair) *Source {
	wsService := client.GetAPIClient().WsService()
	spotPublicWs := wsService.NewSpotPublicWS()

	s := &Source{
		client:       client,
		spotPublicWs: spotPublicWs,
		orderBooks:   make(map[string]*orderbook.OrderBook),
		callbacks:    make([]func(*orderbook.OrderBook), 0),
	}

	// Auto-start if pairs provided
	if len(pairs) > 0 {
		go s.start(pairs)
	}

	return s
}

// start initializes the WebSocket and subscribes to 50-level orderbook
func (s *Source) start(pairs []orderbook.TradingPair) {
	// Start WebSocket connection
	if err := s.spotPublicWs.Start(); err != nil {
		log.Printf("[KuCoin] Failed to start WebSocket: %v", err)
		return
	}
	log.Println("[KuCoin] WebSocket connected")

	// Convert pairs to KuCoin symbol format (BTC-USDT)
	symbols := make([]string, len(pairs))
	for i, p := range pairs {
		symbols[i] = p.Base + "-" + p.Quote
	}

	// Subscribe to Level50 orderbook
	_, err := s.spotPublicWs.OrderbookLevel50(symbols, func(topic string, subject string, data *spotpublic.OrderbookLevel50Event) error {
		symbol := extractSymbolFromTopic(topic)
		base, quote := parseSymbol(symbol)
		pairKey := base + "-" + quote

		ob := &orderbook.OrderBook{
			Source:    SourceName,
			Base:      base,
			Quote:     quote,
			Bids:      convertPriceLevels(data.Bids),
			Asks:      convertPriceLevels(data.Asks),
			Timestamp: data.Timestamp,
			UpdatedAt: time.Now(),
		}

		s.mu.Lock()
		s.orderBooks[pairKey] = ob
		callbacks := s.callbacks
		s.mu.Unlock()

		for _, cb := range callbacks {
			cb(ob)
		}

		return nil
	})

	if err != nil {
		log.Printf("[KuCoin] Failed to subscribe to Level50 orderbook: %v", err)
		return
	}

	log.Printf("[KuCoin] Subscribed to Level50 orderbook for %v", symbols)
}

// Name returns the price source name
func (s *Source) Name() orderbook.PriceSourceName {
	return SourceName
}

// GetOrderBook returns the latest orderbook for a trading pair
func (s *Source) GetOrderBook(base, quote string) *orderbook.OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orderBooks[base+"-"+quote]
}

// GetAllOrderBooks returns all current orderbooks
func (s *Source) GetAllOrderBooks() map[string]*orderbook.OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*orderbook.OrderBook, len(s.orderBooks))
	for k, v := range s.orderBooks {
		result[k] = v
	}
	return result
}

// OnUpdate registers a callback for orderbook updates
func (s *Source) OnUpdate(callback func(*orderbook.OrderBook)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = append(s.callbacks, callback)
}

// Helper functions

func extractSymbolFromTopic(topic string) string {
	for i := len(topic) - 1; i >= 0; i-- {
		if topic[i] == ':' {
			return topic[i+1:]
		}
	}
	return topic
}

func convertPriceLevels(levels [][]string) []orderbook.PriceLevel {
	result := make([]orderbook.PriceLevel, len(levels))
	for i, level := range levels {
		if len(level) >= 2 {
			result[i] = orderbook.PriceLevel{
				Price:    level[0],
				Quantity: level[1],
			}
		}
	}
	return result
}

// parseSymbol extracts base and quote from symbol format like "BTC-USDT"
func parseSymbol(symbol string) (base, quote string) {
	parts := strings.Split(symbol, "-")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return symbol, ""
}

// Ensure Source implements PriceSource interface
var _ orderbook.PriceSource = (*Source)(nil)
