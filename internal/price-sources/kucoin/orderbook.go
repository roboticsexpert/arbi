package kucoin

import (
	"log"
	"sync"
	"time"

	"arbi/internal/orderbook"

	"github.com/Kucoin/kucoin-universal-sdk/sdk/golang/pkg/generate/spot/spotpublic"
)

const ExchangeName = "kucoin"

// Source implements the orderbook.PriceSource interface for KuCoin
// It automatically connects and streams 50-level orderbook data
type Source struct {
	client       *Client
	spotPublicWs spotpublic.SpotPublicWS
	orderBooks   map[string]*orderbook.OrderBook
	callbacks    []func(*orderbook.OrderBook)
	mu           sync.RWMutex
}

// NewSource creates a new KuCoin price source and automatically starts streaming
// orderbook data for the given symbols (50-level depth by default)
func NewSource(client *Client, symbols []string) *Source {
	wsService := client.GetAPIClient().WsService()
	spotPublicWs := wsService.NewSpotPublicWS()

	s := &Source{
		client:       client,
		spotPublicWs: spotPublicWs,
		orderBooks:   make(map[string]*orderbook.OrderBook),
		callbacks:    make([]func(*orderbook.OrderBook), 0),
}

	// Auto-start if symbols provided
	if len(symbols) > 0 {
		go s.start(symbols)
	}

	return s
}

// start initializes the WebSocket and subscribes to 50-level orderbook
func (s *Source) start(symbols []string) {
	// Start WebSocket connection
	if err := s.spotPublicWs.Start(); err != nil {
		log.Printf("[KuCoin] Failed to start WebSocket: %v", err)
		return
	}
	log.Println("[KuCoin] WebSocket connected")

	// Subscribe to Level50 orderbook
	_, err := s.spotPublicWs.OrderbookLevel50(symbols, func(topic string, subject string, data *spotpublic.OrderbookLevel50Event) error {
		symbol := extractSymbolFromTopic(topic)

		ob := &orderbook.OrderBook{
			Exchange:  ExchangeName,
			Symbol:    symbol,
			Bids:      convertPriceLevels(data.Bids),
			Asks:      convertPriceLevels(data.Asks),
			Timestamp: data.Timestamp,
			UpdatedAt: time.Now(),
		}

		s.mu.Lock()
		s.orderBooks[symbol] = ob
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

// Name returns the exchange name
func (s *Source) Name() string {
	return ExchangeName
}

// GetOrderBook returns the latest orderbook for a symbol
func (s *Source) GetOrderBook(symbol string) *orderbook.OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orderBooks[symbol]
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

// Ensure Source implements PriceSource interface
var _ orderbook.PriceSource = (*Source)(nil)
