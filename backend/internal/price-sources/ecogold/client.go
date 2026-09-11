package ecogold

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
	SellPrice string `json:"sell_price"` // Price at which ecogold sells (your buy/ask)
	BuyPrice  string `json:"buy_price"`  // Price at which ecogold buys (your sell/bid)
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

// BalanceItem represents a single balance from /api/balances
type BalanceItem struct {
	CurrencySymbol string `json:"currency_symbol"`
	Value          string `json:"value"`
	LockedValue    string `json:"locked_value"`
}

// BalancesResponse is the response from GET /api/balances
type BalancesResponse struct {
	Data []BalanceItem `json:"data"`
}

// AuthClient handles authenticated EcoGold API requests (balance, verify-password)
type AuthClient struct {
	token      string
	password   string
	httpClient *http.Client
}

// NewAuthClient creates a new EcoGold auth client for balance endpoints
func NewAuthClient(token, password string) *AuthClient {
	return &AuthClient{
		token:    token,
		password: password,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// VerifyPassword POSTs the password to /api/auth/verify-password to keep the token valid
func (c *AuthClient) VerifyPassword() error {
	body, err := json.Marshal(map[string]string{"password": c.password})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, BaseURL+"/api/auth/verify-password", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("verify-password failed: status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// GetBalances fetches wallet balances from GET /api/balances
func (c *AuthClient) GetBalances() (*BalancesResponse, error) {
	req, err := http.NewRequest(http.MethodGet, BaseURL+"/api/balances", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("balances failed: status %d: %s", resp.StatusCode, string(body))
	}

	var result BalancesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result, nil
}

// ParseBalance converts balance string to float64
func ParseBalance(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
