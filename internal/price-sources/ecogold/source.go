package ecogold

import (
	"log"
	"sync"
	"time"

	"arbi/internal/orderbook"
)

const (
	SourceName    = orderbook.PriceSourceEcoGold
	PollInterval  = 30 * time.Second
	DefaultVolume = "1000" // Simulated volume for OTC orderbook
)

// Source implements the orderbook.PriceSource interface for EcoGold
// It polls the REST API every 30 seconds and converts OTC prices to orderbook format
type Source struct {
	client     *Client
	orderBooks map[string]*orderbook.OrderBook
	callbacks  []func(*orderbook.OrderBook)
	mu         sync.RWMutex
	stopCh     chan struct{}
}

// NewSource creates a new EcoGold price source and automatically starts polling
func NewSource() *Source {
	s := &Source{
		client:     NewClient(),
		orderBooks: make(map[string]*orderbook.OrderBook),
		callbacks:  make([]func(*orderbook.OrderBook), 0),
		stopCh:     make(chan struct{}),
	}

	// Auto-start polling
	go s.startPolling()

	return s
}

// startPolling continuously fetches prices from the API
func (s *Source) startPolling() {
	// Fetch immediately on start
	s.fetchAndUpdate()

	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.fetchAndUpdate()
		case <-s.stopCh:
			log.Println("[EcoGold] Polling stopped")
			return
		}
	}
}

// fetchAndUpdate fetches prices and updates orderbooks
func (s *Source) fetchAndUpdate() {
	prices, err := s.client.GetPrices()
	if err != nil {
		log.Printf("[EcoGold] Failed to fetch prices: %v", err)
		return
	}

	for _, price := range prices.Data {
		ob := s.convertToOrderBook(price)

		s.mu.Lock()
		s.orderBooks[price.Symbol] = ob
		callbacks := s.callbacks
		s.mu.Unlock()

		// Notify callbacks
		for _, cb := range callbacks {
			cb(ob)
		}
	}

	log.Printf("[EcoGold] Updated %d prices", len(prices.Data))
}

// convertToOrderBook converts OTC price data to orderbook format
func (s *Source) convertToOrderBook(price PriceData) *orderbook.OrderBook {
	// In OTC:
	// - sell_price = price at which exchange sells to you = ASK (you buy at this price)
	// - buy_price = price at which exchange buys from you = BID (you sell at this price)

	return &orderbook.OrderBook{
		Source: orderbook.PriceSourceEcoGold,
		Symbol: price.Symbol,
		Bids: []orderbook.PriceLevel{
			{
				Price:    price.BuyPrice,
				Quantity: DefaultVolume,
			},
		},
		Asks: []orderbook.PriceLevel{
			{
				Price:    price.SellPrice,
				Quantity: DefaultVolume,
			},
		},
		Timestamp: time.Now().UnixMilli(),
		UpdatedAt: time.Now(),
	}
}

// Name returns the exchange name
func (s *Source) Name() orderbook.PriceSourceName {
	return orderbook.PriceSourceEcoGold
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

// Ensure Source implements PriceSource interface
var _ orderbook.PriceSource = (*Source)(nil)
