package binance

import (
	"arbi/internal/config"

	"github.com/adshao/go-binance/v2"
)

// Client wraps the Binance SDK client
type Client struct {
	client *binance.Client
}

// NewClient creates a new Binance client
// API keys are optional for public endpoints (like order book)
func NewClient() *Client {
	// Retrieve API secret information from environment variables (optional for public data)
	key := config.BINANCE_API_KEY
	secret := config.BINANCE_API_SECRET

	// Set proxy if configured
	if config.PROXY_URI != "" {
		binance.SetWsProxyUrl(config.PROXY_URI)
	}

	// Create client
	client := binance.NewClient(key, secret)

	return &Client{
		client: client,
	}
}

// GetAPIClient returns the underlying Binance API client
func (c *Client) GetAPIClient() *binance.Client {
	return c.client
}
