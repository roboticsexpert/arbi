package ecogold

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	BaseURL = "https://backend.ecogold.ir"
)

// Client handles HTTP requests to EcoGold API
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new EcoGold API client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// PriceResponse represents the API response structure
type PriceResponse struct {
	Data []PriceData `json:"data"`
}

// PriceData represents a single price entry
type PriceData struct {
	Symbol    string `json:"symbol"`
	SellPrice string `json:"buy_price"`  // Price at which ecogold sells (your buy/ask)
	BuyPrice  string `json:"sell_price"` // Price at which ecogold buys (your sell/bid)
	Change    string `json:"change"`
	Signature string `json:"signature"`
	ValidAt   string `json:"valid_at"`
	CreatedAt string `json:"created_at"`
}

// GetPrices fetches current OTC prices from EcoGold
func (c *Client) GetPrices() (*PriceResponse, error) {
	url := BaseURL + "/api/prices/otc"

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch prices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var priceResp PriceResponse
	if err := json.Unmarshal(body, &priceResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &priceResp, nil
}
