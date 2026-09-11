package kucoin

import (
	"net/http"
	"net/url"
	"time"

	"arbi/internal/config"

	"github.com/Kucoin/kucoin-universal-sdk/sdk/golang/pkg/api"
	"github.com/Kucoin/kucoin-universal-sdk/sdk/golang/pkg/common/logger"
	"github.com/Kucoin/kucoin-universal-sdk/sdk/golang/pkg/types"
)

// Client wraps the KuCoin SDK client
type Client struct {
	client *api.DefaultClient
}

// NewClient creates a new KuCoin client
// API keys are optional for public endpoints (like order book)
func NewClient() *Client {
	// Use the default logger or supply your custom logger
	defaultLogger := logger.NewDefaultLogger()
	logger.SetLogger(defaultLogger)

	// Retrieve API secret information from environment variables (optional for public data)
	key := config.KUCOIN_API_KEY
	secret := config.KUCOIN_API_SECRET
	passphrase := config.KUCOIN_API_PASSPHRASE

	// Set WebSocket options with auto-reconnect (high attempts for VPN/network issues)
	wsOption := types.NewWebSocketClientOptionBuilder().
		WithReconnect(true).
		WithReconnectAttempts(100).
		WithReconnectInterval(5 * time.Second).
		Build()

	// Set HTTP transport options
	httpOption := types.NewTransportOptionBuilder().
		SetProxy(func(r *http.Request) (*url.URL, error) {
			// No proxy configured (e.g. running outside Iran) - connect directly.
			if config.PROXY_URI == "" {
				return nil, nil
			}
			return url.Parse(config.PROXY_URI)
		}).
		Build()

	// Create client options
	optionBuilder := types.NewClientOptionBuilder().
		WithSpotEndpoint(types.GlobalApiEndpoint).
		WithFuturesEndpoint(types.GlobalFuturesApiEndpoint).
		WithBrokerEndpoint(types.GlobalBrokerApiEndpoint).
		WithWebSocketClientOption(wsOption).
		WithTransportOption(httpOption)

	// Add API credentials if provided
	if key != "" && secret != "" && passphrase != "" {
		optionBuilder = optionBuilder.
			WithKey(key).
			WithSecret(secret).
			WithPassphrase(passphrase)
	}

	option := optionBuilder.Build()
	client := api.NewClient(option)

	return &Client{
		client: client,
	}
}

// GetAPIClient returns the underlying KuCoin API client
func (c *Client) GetAPIClient() *api.DefaultClient {
	return c.client
}
