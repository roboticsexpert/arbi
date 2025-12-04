package nobitex

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"arbi/internal/orderbook"

	"github.com/centrifugal/centrifuge-go"
)

const (
	SourceName   = orderbook.PriceSourceNobitex
	WebSocketURL = "wss://ws.nobitex.ir/connection/websocket"
)

// OrderbookData represents the orderbook data from Nobitex
type OrderbookData struct {
	Bids [][]string `json:"bids"` // [[price, quantity], ...]
	Asks [][]string `json:"asks"` // [[price, quantity], ...]
}

// Source implements the orderbook.PriceSource interface for Nobitex
type Source struct {
	client       *centrifuge.Client
	subscription *centrifuge.Subscription
	orderBooks   map[string]*orderbook.OrderBook
	callbacks    []func(*orderbook.OrderBook)
	mu           sync.RWMutex
	symbols      []string
}

// NewSource creates a new Nobitex price source and automatically connects
func NewSource(symbols []string) *Source {
	s := &Source{
		orderBooks: make(map[string]*orderbook.OrderBook),
		callbacks:  make([]func(*orderbook.OrderBook), 0),
		symbols:    symbols,
	}

	if len(symbols) > 0 {
		go s.connect()
	}

	return s
}

// connect establishes the WebSocket connection
func (s *Source) connect() {
	s.client = centrifuge.NewJsonClient(WebSocketURL, centrifuge.Config{})

	s.client.OnConnecting(func(e centrifuge.ConnectingEvent) {
		log.Printf("[Nobitex] Connecting... (%s)", e.Reason)
	})

	s.client.OnConnected(func(e centrifuge.ConnectedEvent) {
		log.Printf("[Nobitex] Connected! Client ID: %s", e.ClientID)
	})

	s.client.OnDisconnected(func(e centrifuge.DisconnectedEvent) {
		log.Printf("[Nobitex] Disconnected (%s)", e.Reason)
		// Reconnect after a delay
		time.Sleep(5 * time.Second)
		go s.connect()
	})

	s.client.OnError(func(e centrifuge.ErrorEvent) {
		log.Printf("[Nobitex] Error: %v", e.Error)
	})

	// Subscribe to each symbol
	for _, symbol := range s.symbols {
		if err := s.subscribeToSymbol(symbol); err != nil {
			log.Printf("[Nobitex] Failed to subscribe to %s: %v", symbol, err)
		}
	}

	// Connect to server
	if err := s.client.Connect(); err != nil {
		log.Printf("[Nobitex] Failed to connect: %v", err)
		// Retry after delay
		time.Sleep(5 * time.Second)
		go s.connect()
	}
}

// subscribeToSymbol subscribes to a single symbol's orderbook
func (s *Source) subscribeToSymbol(symbol string) error {
	channelName := fmt.Sprintf("public:orderbook-%s", symbol)
	log.Printf("[Nobitex] Subscribing to channel: %s", channelName)

	sub, err := s.client.NewSubscription(channelName)
	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	sub.OnSubscribing(func(e centrifuge.SubscribingEvent) {
		log.Printf("[Nobitex] Subscribing to %s... (%s)", channelName, e.Reason)
	})

	sub.OnSubscribed(func(e centrifuge.SubscribedEvent) {
		log.Printf("[Nobitex] Subscribed to %s", channelName)
	})

	sub.OnUnsubscribed(func(e centrifuge.UnsubscribedEvent) {
		log.Printf("[Nobitex] Unsubscribed from %s (%s)", channelName, e.Reason)
	})

	sub.OnError(func(e centrifuge.SubscriptionErrorEvent) {
		log.Printf("[Nobitex] Subscription Error: %v", e.Error)
	})

	// Handle orderbook updates
	sub.OnPublication(func(e centrifuge.PublicationEvent) {
		var data OrderbookData
		if err := json.Unmarshal(e.Data, &data); err != nil {
			log.Printf("[Nobitex] Failed to parse orderbook data: %v", err)
			return
		}

		ob := s.convertToOrderBook(symbol, &data)

		s.mu.Lock()
		s.orderBooks[symbol] = ob
		callbacks := s.callbacks
		s.mu.Unlock()

		for _, cb := range callbacks {
			cb(ob)
		}
	})

	if err := sub.Subscribe(); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	return nil
}

// convertToOrderBook converts Nobitex data to orderbook format
// Note: Nobitex prices are in IRR (Rial), we convert to IRT (Toman) by dividing by 10
func (s *Source) convertToOrderBook(symbol string, data *OrderbookData) *orderbook.OrderBook {
	bids := make([]orderbook.PriceLevel, 0, len(data.Bids))
	for _, bid := range data.Bids {
		if len(bid) >= 2 {
			bids = append(bids, orderbook.PriceLevel{
				Price:    convertIRRtoIRT(bid[0]),
				Quantity: bid[1],
			})
		}
	}

	asks := make([]orderbook.PriceLevel, 0, len(data.Asks))
	for _, ask := range data.Asks {
		if len(ask) >= 2 {
			asks = append(asks, orderbook.PriceLevel{
				Price:    convertIRRtoIRT(ask[0]),
				Quantity: ask[1],
			})
		}
	}

	return &orderbook.OrderBook{
		Source:    orderbook.PriceSourceNobitex,
		Symbol:    symbol,
		Bids:      bids,
		Asks:      asks,
		Timestamp: time.Now().UnixMilli(),
		UpdatedAt: time.Now(),
	}
}

// convertIRRtoIRT converts price from Rial (IRR) to Toman (IRT) by dividing by 10
func convertIRRtoIRT(priceStr string) string {
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		log.Printf("[Nobitex] Failed to parse price '%s': %v", priceStr, err)
		return priceStr
	}

	// Convert from Rial to Toman (divide by 10)
	priceInToman := price / 10.0

	// Format with 2 decimal places
	return strconv.FormatFloat(priceInToman, 'f', 2, 64)
}

// Name returns the exchange name
func (s *Source) Name() orderbook.PriceSourceName {
	return orderbook.PriceSourceNobitex
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
