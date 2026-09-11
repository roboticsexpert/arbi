// Package mt5 exposes MetaTrader 5 quotes as a price source.
//
// MetaTrader has no server-side API a backend can call: quotes only exist
// inside a running terminal. So unlike every other source here, this one does
// not poll or dial out — it is a sink. An Expert Advisor running in the
// terminal (see mt5/ArbiPriceFeed.mq5) POSTs ticks to /api/mt5/ticks, and that
// handler calls Push. Swapping the EA for a hosted bridge later changes nothing
// on this side.
package mt5

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"arbi/internal/orderbook"
)

const (
	SourceName = orderbook.PriceSourceMT5

	// DefaultStaleAfter is how long a quote stays usable without a fresh push.
	// Metals CFDs close over the weekend and the EA only runs while the
	// terminal is up, so quotes go quiet for entirely normal reasons.
	DefaultStaleAfter = 90 * time.Second

	// DefaultVolume backs symbols whose spec leaves Volume unset.
	DefaultVolume = "1000"
)

// Tick is one symbol's top-of-book as sent by the Expert Advisor.
type Tick struct {
	Symbol string  `json:"symbol"`
	Bid    float64 `json:"bid"`
	Ask    float64 `json:"ask"`
	// Time is the broker's tick time in unix seconds. Optional: when absent or
	// zero the server's own clock is used.
	Time int64 `json:"time,omitempty"`
}

// PushRequest is the body of POST /api/mt5/ticks.
type PushRequest struct {
	Ticks []Tick `json:"ticks"`
}

// PushResult reports what the server did with each tick, so a misconfigured EA
// shows up as a visible response rather than silence.
type PushResult struct {
	Accepted []string `json:"accepted"`
	Skipped  []string `json:"skipped,omitempty"`
}

// Source implements orderbook.PriceSource for MetaTrader 5.
type Source struct {
	orderBooks map[string]*orderbook.OrderBook // key: "base-quote"
	callbacks  []func(*orderbook.OrderBook)
	staleAfter time.Duration
	stale      map[string]bool
	mu         sync.RWMutex
	stopCh     chan struct{}
}

// NewSource creates the MT5 source and starts the staleness watcher.
func NewSource(staleAfter time.Duration) *Source {
	if staleAfter <= 0 {
		staleAfter = DefaultStaleAfter
	}

	s := &Source{
		orderBooks: make(map[string]*orderbook.OrderBook),
		callbacks:  make([]func(*orderbook.OrderBook), 0),
		staleAfter: staleAfter,
		stale:      make(map[string]bool),
		stopCh:     make(chan struct{}),
	}

	go s.watchStale()

	return s
}

// Push converts incoming ticks to orderbooks and notifies the store.
func (s *Source) Push(ticks []Tick) PushResult {
	result := PushResult{Accepted: make([]string, 0, len(ticks))}

	for _, tick := range ticks {
		spec, ok := LookupSymbol(tick.Symbol)
		if !ok {
			result.Skipped = append(result.Skipped, tick.Symbol+": unknown symbol")
			continue
		}
		if tick.Bid <= 0 || tick.Ask <= 0 {
			result.Skipped = append(result.Skipped, tick.Symbol+": non-positive bid/ask")
			continue
		}
		if spec.Divisor <= 0 {
			result.Skipped = append(result.Skipped, tick.Symbol+": invalid divisor")
			continue
		}

		ob := s.toOrderBook(tick, spec)

		// A tick that is already stale on arrival means the market is closed or
		// the broker clock is skewed. Either way the quote is about to be
		// retired, so say why rather than letting gold quietly never appear.
		if age := time.Since(ob.UpdatedAt); age > s.staleAfter {
			log.Printf("[MT5] %s tick arrived already %v old (market closed, or broker clock skew)", tick.Symbol, age.Round(time.Second))
		}

		s.publish(ob)
		result.Accepted = append(result.Accepted, tick.Symbol)
	}

	return result
}

// toOrderBook builds a single-level orderbook from a tick.
func (s *Source) toOrderBook(tick Tick, spec SymbolSpec) *orderbook.OrderBook {
	// Staleness is measured against the broker's tick time, not our receive
	// time. The EA heartbeats even when nothing moves, so receive time would
	// make a frozen weekend quote look perpetually fresh.
	updatedAt := time.Now()
	if tick.Time > 0 {
		updatedAt = time.Unix(tick.Time, 0)
	}

	volume := spec.Volume
	if volume == "" {
		volume = DefaultVolume
	}

	return &orderbook.OrderBook{
		Source: orderbook.PriceSourceMT5,
		Base:   spec.Base,
		Quote:  spec.Quote,
		// Bid is what the broker pays us, ask is what we pay the broker —
		// same convention as every other source here.
		Bids: []orderbook.PriceLevel{{
			Price:    formatPrice(tick.Bid / spec.Divisor),
			Quantity: volume,
		}},
		Asks: []orderbook.PriceLevel{{
			Price:    formatPrice(tick.Ask / spec.Divisor),
			Quantity: volume,
		}},
		Timestamp: updatedAt.UnixMilli(),
		UpdatedAt: updatedAt,
	}
}

// publish stores an orderbook and fans it out to registered callbacks.
func (s *Source) publish(ob *orderbook.OrderBook) {
	pairKey := ob.Base + "-" + ob.Quote

	s.mu.Lock()
	s.orderBooks[pairKey] = ob
	s.stale[pairKey] = false
	callbacks := s.callbacks
	s.mu.Unlock()

	for _, cb := range callbacks {
		cb(ob)
	}
}

// watchStale empties the price levels of quotes that have stopped arriving.
//
// The central store keeps the last book it was handed, so simply withholding
// updates would leave a stale gold price in place, looking live to the
// arbitrage finder for as long as the terminal stays down. Publishing a book
// with no levels retires it from the graph — buildGraph only emits edges for
// books that still have bids or asks — while leaving the timestamp visible.
func (s *Source) watchStale() {
	interval := s.staleAfter / 3
	if interval < time.Second {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.retireStale()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Source) retireStale() {
	now := time.Now()

	s.mu.Lock()
	var retired []*orderbook.OrderBook
	for pairKey, ob := range s.orderBooks {
		if s.stale[pairKey] || now.Sub(ob.UpdatedAt) <= s.staleAfter {
			continue
		}

		empty := &orderbook.OrderBook{
			Source:    ob.Source,
			Base:      ob.Base,
			Quote:     ob.Quote,
			Bids:      []orderbook.PriceLevel{},
			Asks:      []orderbook.PriceLevel{},
			Timestamp: ob.Timestamp,
			UpdatedAt: ob.UpdatedAt,
		}
		s.orderBooks[pairKey] = empty
		s.stale[pairKey] = true
		retired = append(retired, empty)
	}
	callbacks := s.callbacks
	s.mu.Unlock()

	for _, ob := range retired {
		log.Printf("[MT5] %s went stale (no tick for %v), dropping price levels", ob.Pair(), s.staleAfter)
		for _, cb := range callbacks {
			cb(ob)
		}
	}
}

// Stop halts the staleness watcher.
func (s *Source) Stop() {
	close(s.stopCh)
}

// formatPrice renders a price without exponent or trailing zeroes.
func formatPrice(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// Name returns the price source name.
func (s *Source) Name() orderbook.PriceSourceName {
	return orderbook.PriceSourceMT5
}

// GetOrderBook returns the latest orderbook for a trading pair.
func (s *Source) GetOrderBook(base, quote string) *orderbook.OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orderBooks[base+"-"+quote]
}

// GetAllOrderBooks returns all current orderbooks.
func (s *Source) GetAllOrderBooks() map[string]*orderbook.OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*orderbook.OrderBook, len(s.orderBooks))
	for k, v := range s.orderBooks {
		result[k] = v
	}
	return result
}

// OnUpdate registers a callback for orderbook updates.
func (s *Source) OnUpdate(callback func(*orderbook.OrderBook)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = append(s.callbacks, callback)
}

// Describe returns a human-readable summary used by the ingest handler's logs.
func (s *Source) Describe() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf("%d pairs, stale after %v", len(s.orderBooks), s.staleAfter)
}

// Ensure Source implements PriceSource interface
var _ orderbook.PriceSource = (*Source)(nil)
