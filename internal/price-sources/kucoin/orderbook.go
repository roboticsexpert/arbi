package kucoin

import (
	"fmt"
	"sync"

	"github.com/Kucoin/kucoin-universal-sdk/sdk/golang/pkg/common/logger"
	"github.com/Kucoin/kucoin-universal-sdk/sdk/golang/pkg/generate/spot/spotpublic"
)

// PriceLevel represents a single price level in the order book
type PriceLevel struct {
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
}

// OrderBook represents the current state of an order book
type OrderBook struct {
	Symbol    string       `json:"symbol"`
	Bids      []PriceLevel `json:"bids"`
	Asks      []PriceLevel `json:"asks"`
	Timestamp int64        `json:"timestamp"`
}

// OrderBookService manages WebSocket connections for order book data
type OrderBookService struct {
	client        *Client
	spotPublicWs  spotpublic.SpotPublicWS
	subscriptions map[string]string // symbol -> subscription ID
	orderBooks    map[string]*OrderBook
	callbacks     map[string][]func(*OrderBook)
	mu            sync.RWMutex
	started       bool
}

// NewOrderBookService creates a new order book service
func NewOrderBookService(client *Client) *OrderBookService {
	wsService := client.GetAPIClient().WsService()
	spotPublicWs := wsService.NewSpotPublicWS()

	return &OrderBookService{
		client:        client,
		spotPublicWs:  spotPublicWs,
		subscriptions: make(map[string]string),
		orderBooks:    make(map[string]*OrderBook),
		callbacks:     make(map[string][]func(*OrderBook)),
	}
}

// Start starts the WebSocket connection
func (s *OrderBookService) Start() error {
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
func (s *OrderBookService) Stop() error {
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

// SubscribeLevel5 subscribes to Level 5 order book updates (top 5 bids/asks)
// This is the most commonly used level for trading applications
func (s *OrderBookService) SubscribeLevel5(symbols []string, callback func(*OrderBook)) error {
	if err := s.Start(); err != nil {
		return err
	}

	s.mu.Lock()
	for _, symbol := range symbols {
		s.callbacks[symbol] = append(s.callbacks[symbol], callback)
	}
	s.mu.Unlock()

	subId, err := s.spotPublicWs.OrderbookLevel5(symbols, func(topic string, subject string, data *spotpublic.OrderbookLevel5Event) error {
		// Extract symbol from topic (format: /spotMarket/level2Depth5:BTC-USDT)
		symbol := extractSymbolFromTopic(topic)

		orderBook := &OrderBook{
			Symbol:    symbol,
			Bids:      convertPriceLevels(data.Bids),
			Asks:      convertPriceLevels(data.Asks),
			Timestamp: data.Timestamp,
		}

		s.mu.Lock()
		s.orderBooks[symbol] = orderBook
		callbacks := s.callbacks[symbol]
		s.mu.Unlock()

		// Call all registered callbacks
		for _, cb := range callbacks {
			cb(orderBook)
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
func (s *OrderBookService) SubscribeLevel50(symbols []string, callback func(*OrderBook)) error {
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

		orderBook := &OrderBook{
			Symbol:    symbol,
			Bids:      convertPriceLevels(data.Bids),
			Asks:      convertPriceLevels(data.Asks),
			Timestamp: data.Timestamp,
		}

		s.mu.Lock()
		s.orderBooks[symbol] = orderBook
		callbacks := s.callbacks[symbol]
		s.mu.Unlock()

		for _, cb := range callbacks {
			cb(orderBook)
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

// SubscribeIncrement subscribes to incremental order book updates
func (s *OrderBookService) SubscribeIncrement(symbols []string, callback func(*OrderBookIncrement)) error {
	if err := s.Start(); err != nil {
		return err
	}

	subId, err := s.spotPublicWs.OrderbookIncrement(symbols, func(topic string, subject string, data *spotpublic.OrderbookIncrementEvent) error {
		symbol := extractSymbolFromTopic(topic)

		increment := &OrderBookIncrement{
			Symbol:        symbol,
			SequenceStart: data.SequenceStart,
			SequenceEnd:   data.SequenceEnd,
			Time:          data.Time,
			Changes: OrderBookChanges{
				Asks: convertIncrementLevels(data.Changes.Asks),
				Bids: convertIncrementLevels(data.Changes.Bids),
			},
		}

		callback(increment)
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to increment order book: %w", err)
	}

	s.mu.Lock()
	for _, symbol := range symbols {
		s.subscriptions[symbol+"_increment"] = subId
	}
	s.mu.Unlock()

	logger.GetLogger().Infof("Subscribed to incremental order book for %v", symbols)
	return nil
}

// Unsubscribe unsubscribes from order book updates for a symbol
func (s *OrderBookService) Unsubscribe(symbol string, level string) error {
	s.mu.Lock()
	key := symbol + "_" + level
	subId, exists := s.subscriptions[key]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("no subscription found for %s at %s level", symbol, level)
	}
	delete(s.subscriptions, key)
	s.mu.Unlock()

	err := s.spotPublicWs.UnSubscribe(subId)
	if err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}

	logger.GetLogger().Infof("Unsubscribed from %s order book for %s", level, symbol)
	return nil
}

// GetOrderBook returns the latest order book for a symbol
func (s *OrderBookService) GetOrderBook(symbol string) *OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orderBooks[symbol]
}

// GetAllOrderBooks returns all current order books
func (s *OrderBookService) GetAllOrderBooks() map[string]*OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*OrderBook)
	for k, v := range s.orderBooks {
		result[k] = v
	}
	return result
}

// OrderBookIncrement represents an incremental order book update
type OrderBookIncrement struct {
	Symbol        string           `json:"symbol"`
	SequenceStart int64            `json:"sequenceStart"`
	SequenceEnd   int64            `json:"sequenceEnd"`
	Time          int64            `json:"time"`
	Changes       OrderBookChanges `json:"changes"`
}

// OrderBookChanges represents the changes in an incremental update
type OrderBookChanges struct {
	Asks []IncrementLevel `json:"asks"`
	Bids []IncrementLevel `json:"bids"`
}

// IncrementLevel represents a single increment update
type IncrementLevel struct {
	Price    string `json:"price"`
	Size     string `json:"size"`
	Sequence string `json:"sequence"`
}

// Helper functions

func extractSymbolFromTopic(topic string) string {
	// Topic format: /spotMarket/level2Depth5:BTC-USDT or /market/level2:BTC-USDT
	for i := len(topic) - 1; i >= 0; i-- {
		if topic[i] == ':' {
			return topic[i+1:]
		}
	}
	return topic
}

func convertPriceLevels(levels [][]string) []PriceLevel {
	result := make([]PriceLevel, len(levels))
	for i, level := range levels {
		if len(level) >= 2 {
			result[i] = PriceLevel{
				Price:    level[0],
				Quantity: level[1],
			}
		}
	}
	return result
}

func convertIncrementLevels(levels [][]string) []IncrementLevel {
	result := make([]IncrementLevel, len(levels))
	for i, level := range levels {
		if len(level) >= 3 {
			result[i] = IncrementLevel{
				Price:    level[0],
				Size:     level[1],
				Sequence: level[2],
			}
		}
	}
	return result
}
