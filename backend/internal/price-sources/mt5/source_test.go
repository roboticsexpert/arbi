package mt5

import (
	"testing"
	"time"

	"arbi/internal/orderbook"
)

func TestPushConvertsKilogramToGramPrice(t *testing.T) {
	s := NewSource(time.Minute)
	defer s.Stop()

	// Real quote observed on InternationalTrading-Server.
	res := s.Push([]Tick{{Symbol: "GOLD_kilogram", Bid: 141209, Ask: 141212}})

	if len(res.Accepted) != 1 || len(res.Skipped) != 0 {
		t.Fatalf("expected 1 accepted 0 skipped, got %+v", res)
	}

	ob := s.GetOrderBook("GOLD24", "USD")
	if ob == nil {
		t.Fatal("GOLD24-USD orderbook missing")
	}
	if got := ob.BestBid().Price; got != "141.209" {
		t.Errorf("bid = %q, want 141.209", got)
	}
	if got := ob.BestAsk().Price; got != "141.212" {
		t.Errorf("ask = %q, want 141.212", got)
	}
	if ob.Source != orderbook.PriceSourceMT5 {
		t.Errorf("source = %q, want mt5", ob.Source)
	}
}

func TestPushKeepsOunceSeparateFromGram(t *testing.T) {
	s := NewSource(time.Minute)
	defer s.Stop()

	s.Push([]Tick{
		{Symbol: "GOLD_kilogram", Bid: 141209, Ask: 141212},
		{Symbol: "GOLD_ounce", Bid: 4392.12, Ask: 4392.20},
	})

	if s.GetOrderBook("GOLD24", "USD") == nil {
		t.Error("GOLD24-USD missing")
	}
	if s.GetOrderBook("XAU", "USD") == nil {
		t.Error("XAU-USD missing")
	}
}

func TestPushRejectsBadTicks(t *testing.T) {
	s := NewSource(time.Minute)
	defer s.Stop()

	res := s.Push([]Tick{
		{Symbol: "XAUUSD", Bid: 1, Ask: 2},             // not a Brachium symbol
		{Symbol: "GOLD_kilogram", Bid: 0, Ask: 141212}, // no bid
		{Symbol: "GOLD_ounce", Bid: -1, Ask: -1},       // nonsense
	})

	if len(res.Accepted) != 0 {
		t.Errorf("accepted = %v, want none", res.Accepted)
	}
	if len(res.Skipped) != 3 {
		t.Errorf("skipped = %v, want 3 entries", res.Skipped)
	}
	if s.GetOrderBook("GOLD24", "USD") != nil {
		t.Error("a rejected tick still created an orderbook")
	}
}

// A stale quote must lose its price levels, otherwise the arbitrage finder
// keeps trading against a frozen price after the terminal goes down.
func TestStaleQuoteLosesPriceLevels(t *testing.T) {
	s := NewSource(time.Minute)
	defer s.Stop()

	var seen []*orderbook.OrderBook
	s.OnUpdate(func(ob *orderbook.OrderBook) { seen = append(seen, ob) })

	s.Push([]Tick{{Symbol: "GOLD_kilogram", Bid: 141209, Ask: 141212}})

	// Backdate the quote past the staleness window.
	s.mu.Lock()
	s.orderBooks["GOLD24-USD"].UpdatedAt = time.Now().Add(-2 * time.Minute)
	s.mu.Unlock()

	s.retireStale()

	ob := s.GetOrderBook("GOLD24", "USD")
	if len(ob.Bids) != 0 || len(ob.Asks) != 0 {
		t.Errorf("stale book still has levels: %+v", ob)
	}
	if len(seen) != 2 {
		t.Fatalf("expected a push and a retirement callback, got %d", len(seen))
	}

	// A fresh tick must bring it back.
	s.Push([]Tick{{Symbol: "GOLD_kilogram", Bid: 141300, Ask: 141303}})
	if got := s.GetOrderBook("GOLD24", "USD").BestBid().Price; got != "141.3" {
		t.Errorf("recovered bid = %q, want 141.3", got)
	}
}

// Retiring must happen once, not on every sweep.
func TestStaleRetirementIsNotRepeated(t *testing.T) {
	s := NewSource(time.Minute)
	defer s.Stop()

	s.Push([]Tick{{Symbol: "GOLD_kilogram", Bid: 141209, Ask: 141212}})

	var callbacks int
	s.OnUpdate(func(ob *orderbook.OrderBook) { callbacks++ })

	s.mu.Lock()
	s.orderBooks["GOLD24-USD"].UpdatedAt = time.Now().Add(-2 * time.Minute)
	s.mu.Unlock()

	s.retireStale()
	s.retireStale()
	s.retireStale()

	if callbacks != 1 {
		t.Errorf("retirement fired %d times, want 1", callbacks)
	}
}
