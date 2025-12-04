package binance

import (
	"log"
	"strings"
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
	orderBooks map[string]*orderbook.OrderBook
	callbacks  []func(*orderbook.OrderBook)
	stopChans  []chan struct{}
	mu         sync.RWMutex
}

// NewSource creates a new Binance price source and automatically starts streaming
// orderbook data for the given symbols (20-level depth by default)
func NewSource(client *Client, symbols []string) *Source {
	s := &Source{
		client:     client,
		orderBooks: make(map[string]*orderbook.OrderBook),
		callbacks:  make([]func(*orderbook.OrderBook), 0),
		stopChans:  make([]chan struct{}, 0),
	}

	// Auto-start if symbols provided
	if len(symbols) > 0 {
		go s.start(symbols)
	}

	return s
}

// start initializes the WebSocket and subscribes to orderbook depth
func (s *Source) start(symbols []string) {
	for _, symbol := range symbols {
		go s.subscribeSymbol(symbol)
	}
}

// subscribeSymbol subscribes to depth updates for a single symbol
func (s *Source) subscribeSymbol(symbol string) {
	// Convert symbol format: BTC-USDT -> BTCUSDT (Binance format)
	binanceSymbol := convertToBinanceSymbol(symbol)

	wsDepthHandler := func(event *binance.WsPartialDepthEvent) {
		ob := &orderbook.OrderBook{
			Source:    SourceName,
			Symbol:    symbol, // Keep original format for consistency
			Bids:      convertBids(event.Bids),
			Asks:      convertAsks(event.Asks),
			Timestamp: event.LastUpdateID,
			UpdatedAt: time.Now(),
		}

		s.mu.Lock()
		s.orderBooks[symbol] = ob
		callbacks := s.callbacks
		s.mu.Unlock()

		for _, cb := range callbacks {
			cb(ob)
		}
	}

	errHandler := func(err error) {
		log.Printf("[Binance] WebSocket error for %s: %v", symbol, err)
	}

	// Use WsPartialDepthServe100Ms for 20-level depth with 100ms updates
	doneC, stopC, err := binance.WsPartialDepthServe100Ms(binanceSymbol, "20", wsDepthHandler, errHandler)
	if err != nil {
		log.Printf("[Binance] Failed to subscribe to depth for %s: %v", symbol, err)
		return
	}

	log.Printf("[Binance] Subscribed to depth for %s", symbol)

	s.mu.Lock()
	s.stopChans = append(s.stopChans, stopC)
	s.mu.Unlock()

	<-doneC
}

// Name returns the price source name
func (s *Source) Name() orderbook.PriceSourceName {
	return SourceName
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

// Stop closes all WebSocket connections
func (s *Source) Stop() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, stopC := range s.stopChans {
		stopC <- struct{}{}
	}
}

// Helper functions

// convertToBinanceSymbol converts symbol format from BTC-USDT to BTCUSDT
func convertToBinanceSymbol(symbol string) string {
	return strings.ReplaceAll(symbol, "-", "")
}

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
