package orderbook

import (
	"log"
	"sync"
)

// Store is the central repository for all orderbooks from all exchanges
type Store struct {
	sources    map[string]PriceSource      // exchange name -> source
	orderbooks map[OrderBookKey]*OrderBook // all orderbooks
	callbacks  []func(key OrderBookKey, ob *OrderBook)
	mu         sync.RWMutex
}

// NewStore creates a new central orderbook store
func NewStore() *Store {
	return &Store{
		sources:    make(map[string]PriceSource),
		orderbooks: make(map[OrderBookKey]*OrderBook),
		callbacks:  make([]func(key OrderBookKey, ob *OrderBook), 0),
	}
}

// AddSource adds a price source to the store
// The source is already running and streaming data
func (s *Store) AddSource(source PriceSource) {
	s.mu.Lock()
	name := source.Name()
	s.sources[name] = source
	s.mu.Unlock()

	// Register callback to update central store
	source.OnUpdate(func(ob *OrderBook) {
		key := OrderBookKey{Exchange: name, Symbol: ob.Symbol}

		s.mu.Lock()
		s.orderbooks[key] = ob
		callbacks := s.callbacks
		s.mu.Unlock()

		// Notify all global callbacks
		for _, cb := range callbacks {
			cb(key, ob)
		}
	})

	log.Printf("[Store] Added price source: %s", name)
}

// OnUpdate registers a callback for orderbook updates
func (s *Store) OnUpdate(callback func(key OrderBookKey, ob *OrderBook)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = append(s.callbacks, callback)
}

// Get returns the orderbook for a specific exchange and symbol
func (s *Store) Get(exchange, symbol string) *OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orderbooks[OrderBookKey{Exchange: exchange, Symbol: symbol}]
}

// GetByKey returns the orderbook for a specific key
func (s *Store) GetByKey(key OrderBookKey) *OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orderbooks[key]
}

// GetAll returns all orderbooks
func (s *Store) GetAll() map[OrderBookKey]*OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[OrderBookKey]*OrderBook, len(s.orderbooks))
	for k, v := range s.orderbooks {
		result[k] = v
	}
	return result
}

// GetByExchange returns all orderbooks for a specific exchange
func (s *Store) GetByExchange(exchange string) map[string]*OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*OrderBook)
	for k, v := range s.orderbooks {
		if k.Exchange == exchange {
			result[k.Symbol] = v
		}
	}
	return result
}

// GetBySymbol returns orderbooks for a symbol across all exchanges
func (s *Store) GetBySymbol(symbol string) map[string]*OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*OrderBook)
	for k, v := range s.orderbooks {
		if k.Symbol == symbol {
			result[k.Exchange] = v
		}
	}
	return result
}

// GetSources returns all registered source names
func (s *Store) GetSources() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.sources))
	for name := range s.sources {
		names = append(names, name)
	}
	return names
}

// Stats returns statistics about the store
func (s *Store) Stats() StoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := StoreStats{
		TotalOrderbooks:   len(s.orderbooks),
		RegisteredSources: len(s.sources),
		SourceStats:       make(map[string]SourceStats),
	}

	for name, source := range s.sources {
		stats.SourceStats[name] = SourceStats{
			Orderbooks: len(source.GetAllOrderBooks()),
		}
	}

	return stats
}

// StoreStats holds statistics about the store
type StoreStats struct {
	TotalOrderbooks   int                    `json:"total_orderbooks"`
	RegisteredSources int                    `json:"registered_sources"`
	SourceStats       map[string]SourceStats `json:"source_stats"`
}

// SourceStats holds statistics for a single source
type SourceStats struct {
	Orderbooks int `json:"orderbooks"`
}
