package orderbook

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Store is the central repository for all orderbooks from all exchanges
type Store struct {
	sources    map[string]PriceSource       // exchange name -> source
	orderbooks map[OrderBookKey]*OrderBook  // all orderbooks
	callbacks  []func(key OrderBookKey, ob *OrderBook) // global update callbacks
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewStore creates a new central orderbook store
func NewStore() *Store {
	ctx, cancel := context.WithCancel(context.Background())
	return &Store{
		sources:    make(map[string]PriceSource),
		orderbooks: make(map[OrderBookKey]*OrderBook),
		callbacks:  make([]func(key OrderBookKey, ob *OrderBook), 0),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// RegisterSource adds a price source to the store
func (s *Store) RegisterSource(source PriceSource) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := source.Name()
	if _, exists := s.sources[name]; exists {
		return fmt.Errorf("source %s already registered", name)
	}

	s.sources[name] = source
	log.Printf("[Store] Registered price source: %s", name)
	return nil
}

// Subscribe subscribes to orderbook updates for symbols on a specific exchange
func (s *Store) Subscribe(exchange string, symbols []string) error {
	s.mu.RLock()
	source, exists := s.sources[exchange]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("exchange %s not registered", exchange)
	}

	// Create callback that updates the central store
	callback := func(ob *OrderBook) {
		ob.Exchange = exchange
		ob.UpdatedAt = time.Now()
		
		key := OrderBookKey{Exchange: exchange, Symbol: ob.Symbol}
		
		s.mu.Lock()
		s.orderbooks[key] = ob
		callbacks := s.callbacks
		s.mu.Unlock()

		// Notify all global callbacks
		for _, cb := range callbacks {
			cb(key, ob)
		}
	}

	return source.Subscribe(s.ctx, symbols, callback)
}

// SubscribeAll subscribes to the given symbols on all registered exchanges
func (s *Store) SubscribeAll(symbols []string) error {
	s.mu.RLock()
	sources := make([]PriceSource, 0, len(s.sources))
	for _, src := range s.sources {
		sources = append(sources, src)
	}
	s.mu.RUnlock()

	var errs []error
	for _, source := range sources {
		if err := s.Subscribe(source.Name(), symbols); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", source.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("subscription errors: %v", errs)
	}
	return nil
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

// StartAll starts all registered sources
func (s *Store) StartAll() error {
	s.mu.RLock()
	sources := make([]PriceSource, 0, len(s.sources))
	for _, src := range s.sources {
		sources = append(sources, src)
	}
	s.mu.RUnlock()

	var errs []error
	for _, source := range sources {
		if err := source.Start(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", source.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("start errors: %v", errs)
	}
	return nil
}

// StopAll stops all registered sources and cleans up
func (s *Store) StopAll() error {
	s.cancel()

	s.mu.RLock()
	sources := make([]PriceSource, 0, len(s.sources))
	for _, src := range s.sources {
		sources = append(sources, src)
	}
	s.mu.RUnlock()

	var errs []error
	for _, source := range sources {
		if err := source.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", source.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %v", errs)
	}
	return nil
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
			Running:     source.IsRunning(),
			Orderbooks:  len(source.GetAllOrderBooks()),
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
	Running    bool `json:"running"`
	Orderbooks int  `json:"orderbooks"`
}

