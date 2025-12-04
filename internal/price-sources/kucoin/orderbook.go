package kucoin

import (
	"context"
	"fmt"
	"sync"
	"time"

	"arbi/internal/orderbook"

	"github.com/Kucoin/kucoin-universal-sdk/sdk/golang/pkg/common/logger"
	"github.com/Kucoin/kucoin-universal-sdk/sdk/golang/pkg/generate/spot/spotpublic"
)

const ExchangeName = "kucoin"

// Source implements the orderbook.PriceSource interface for KuCoin
type Source struct {
	client        *Client
	spotPublicWs  spotpublic.SpotPublicWS
	subscriptions map[string]string // symbol -> subscription ID
	orderBooks    map[string]*orderbook.OrderBook
	callbacks     map[string][]func(*orderbook.OrderBook)
	mu            sync.RWMutex
	started       bool
}

// NewSource creates a new KuCoin price source
func NewSource(client *Client) *Source {
	wsService := client.GetAPIClient().WsService()
	spotPublicWs := wsService.NewSpotPublicWS()

	return &Source{
		client:        client,
		spotPublicWs:  spotPublicWs,
		subscriptions: make(map[string]string),
		orderBooks:    make(map[string]*orderbook.OrderBook),
		callbacks:     make(map[string][]func(*orderbook.OrderBook)),
	}
}

// Name returns the exchange name
func (s *Source) Name() string {
	return ExchangeName
}

// Start starts the WebSocket connection
func (s *Source) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return nil
	}

	err := s.spotPublicWs.Start()
	if err != nil {
		return fmt.Errorf("failed to start WebSocket: %w", err)
	}

	s.started = true
	logger.GetLogger().Info("KuCoin WebSocket started")
	return nil
}

// Stop stops the WebSocket connection
func (s *Source) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	err := s.spotPublicWs.Stop()
	if err != nil {
		return fmt.Errorf("failed to stop WebSocket: %w", err)
	}

	s.started = false
	s.subscriptions = make(map[string]string)
	logger.GetLogger().Info("KuCoin WebSocket stopped")
	return nil
}

// IsRunning returns true if the WebSocket is connected
func (s *Source) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.started
}

// Subscribe subscribes to orderbook updates for the given symbols
func (s *Source) Subscribe(ctx context.Context, symbols []string, callback func(*orderbook.OrderBook)) error {
	return s.SubscribeLevel5(symbols, callback)
}

// SubscribeLevel5 subscribes to Level 5 order book updates (top 5 bids/asks)
func (s *Source) SubscribeLevel5(symbols []string, callback func(*orderbook.OrderBook)) error {
	if err := s.Start(); err != nil {
		return err
	}

	s.mu.Lock()
	for _, symbol := range symbols {
		s.callbacks[symbol] = append(s.callbacks[symbol], callback)
	}
	s.mu.Unlock()

	subId, err := s.spotPublicWs.OrderbookLevel5(symbols, func(topic string, subject string, data *spotpublic.OrderbookLevel5Event) error {
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
		callbacks := s.callbacks[symbol]
		s.mu.Unlock()

		for _, cb := range callbacks {
			cb(ob)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to Level5 order book: %w", err)
	}

	s.mu.Lock()
	for _, symbol := range symbols {
		s.subscriptions[symbol+"_level5"] = subId
	}
	s.mu.Unlock()

	logger.GetLogger().Infof("Subscribed to Level5 order book for %v", symbols)
	return nil
}

// SubscribeLevel50 subscribes to Level 50 order book updates (top 50 bids/asks)
func (s *Source) SubscribeLevel50(symbols []string, callback func(*orderbook.OrderBook)) error {
	if err := s.Start(); err != nil {
		return err
	}

	s.mu.Lock()
	for _, symbol := range symbols {
		s.callbacks[symbol] = append(s.callbacks[symbol], callback)
	}
	s.mu.Unlock()

	subId, err := s.spotPublicWs.OrderbookLevel50(symbols, func(topic string, subject string, data *spotpublic.OrderbookLevel50Event) error {
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
		callbacks := s.callbacks[symbol]
		s.mu.Unlock()

		for _, cb := range callbacks {
			cb(ob)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to Level50 order book: %w", err)
	}

	s.mu.Lock()
	for _, symbol := range symbols {
		s.subscriptions[symbol+"_level50"] = subId
	}
	s.mu.Unlock()

	logger.GetLogger().Infof("Subscribed to Level50 order book for %v", symbols)
	return nil
}

// Unsubscribe unsubscribes from order book updates for a symbol
func (s *Source) Unsubscribe(symbol string) error {
	s.mu.Lock()

	// Try to unsubscribe from all levels
	var subId string
	var key string
	for _, level := range []string{"level5", "level50", "increment"} {
		k := symbol + "_" + level
		if id, exists := s.subscriptions[k]; exists {
			subId = id
			key = k
			break
		}
	}

	if subId == "" {
		s.mu.Unlock()
		return fmt.Errorf("no subscription found for %s", symbol)
	}

	delete(s.subscriptions, key)
	delete(s.callbacks, symbol)
	delete(s.orderBooks, symbol)
	s.mu.Unlock()

	err := s.spotPublicWs.UnSubscribe(subId)
	if err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}

	logger.GetLogger().Infof("Unsubscribed from order book for %s", symbol)
	return nil
}

// GetOrderBook returns the latest order book for a symbol
func (s *Source) GetOrderBook(symbol string) *orderbook.OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orderBooks[symbol]
}

// GetAllOrderBooks returns all current order books
func (s *Source) GetAllOrderBooks() map[string]*orderbook.OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*orderbook.OrderBook)
	for k, v := range s.orderBooks {
		result[k] = v
	}
	return result
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
