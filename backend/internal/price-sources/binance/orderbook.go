package binance

import (
	"log"
	"sync"
	"time"

	"arbi/internal/orderbook"

	"github.com/adshao/go-binance/v2"
)

const SourceName = orderbook.PriceSourceBinance

// Source implements the orderbook.PriceSource interface for Binance
// It automatically connects and streams orderbook data
type Source struct {
	client     *Client
	orderBooks map[string]*orderbook.OrderBook // key: "base-quote"
	callbacks  []func(*orderbook.OrderBook)
	stopChans  []chan struct{}
	mu         sync.RWMutex
}

// NewSource creates a new Binance price source and automatically starts streaming
// orderbook data for the given pairs (20-level depth by default)
// pairs format: []orderbook.TradingPair{{Base: "BTC", Quote: "USDT"}, ...}
func NewSource(client *Client, pairs []orderbook.TradingPair) *Source {
	s := &Source{
		client:     client,
		orderBooks: make(map[string]*orderbook.OrderBook),
		callbacks:  make([]func(*orderbook.OrderBook), 0),
		stopChans:  make([]chan struct{}, 0),
	}

	// Auto-start if pairs provided
	if len(pairs) > 0 {
		go s.start(pairs)
	}

	return s
}

// start initializes the WebSocket and subscribes to orderbook depth
func (s *Source) start(pairs []orderbook.TradingPair) {
	for _, pair := range pairs {
		go s.subscribePair(pair)
	}
}

// subscribePair subscribes to depth updates for a single trading pair
func (s *Source) subscribePair(pair orderbook.TradingPair) {
	// Convert to Binance format: BTC + USDT -> BTCUSDT
	binanceSymbol := pair.Base + pair.Quote
	pairKey := pair.String()

	wsDepthHandler := func(event *binance.WsPartialDepthEvent) {
		ob := &orderbook.OrderBook{
			Source:    SourceName,
			Base:      pair.Base,
			Quote:     pair.Quote,
			Bids:      convertBids(event.Bids),
			Asks:      convertAsks(event.Asks),
			Timestamp: event.LastUpdateID,
			UpdatedAt: time.Now(),
		}

		s.mu.Lock()
		s.orderBooks[pairKey] = ob
		callbacks := s.callbacks
		s.mu.Unlock()

		for _, cb := range callbacks {
			cb(ob)
		}
	}

	errHandler := func(err error) {
		log.Printf("[Binance] WebSocket error for %s: %v", pairKey, err)
	}

	// Use WsPartialDepthServe100Ms for 20-level depth with 100ms updates
	doneC, stopC, err := binance.WsPartialDepthServe100Ms(binanceSymbol, "20", wsDepthHandler, errHandler)
	if err != nil {
		log.Printf("[Binance] Failed to subscribe to depth for %s: %v", pairKey, err)
		return
	}

	log.Printf("[Binance] Subscribed to depth for %s", pairKey)

	s.mu.Lock()
	s.stopChans = append(s.stopChans, stopC)
	s.mu.Unlock()

	<-doneC
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

// Stop closes all WebSocket connections
func (s *Source) Stop() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, stopC := range s.stopChans {
		stopC <- struct{}{}
	}
}

// Helper functions

func convertBids(bids []binance.Bid) []orderbook.PriceLevel {
	result := make([]orderbook.PriceLevel, len(bids))
	for i, bid := range bids {
		result[i] = orderbook.PriceLevel{
			Price:    bid.Price,
			Quantity: bid.Quantity,
		}
	}
	return result
}

func convertAsks(asks []binance.Ask) []orderbook.PriceLevel {
	result := make([]orderbook.PriceLevel, len(asks))
	for i, ask := range asks {
		result[i] = orderbook.PriceLevel{
			Price:    ask.Price,
			Quantity: ask.Quantity,
		}
	}
	return result
}

// Ensure Source implements PriceSource interface
var _ orderbook.PriceSource = (*Source)(nil)
