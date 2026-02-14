package nobitex

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	APIBaseURL = "https://apiv2.nobitex.ir"
	UserAgent  = "TraderBot/Arbi"
)

// Wallet represents a single wallet from Nobitex API
type Wallet struct {
	ID             int    `json:"id"`
	Currency       string `json:"currency"`
	Balance        string `json:"balance"`
	BlockedBalance string `json:"blockedBalance"`
	ActiveBalance  string `json:"activeBalance"`
}

// WalletsListResponse is the response from GET /users/wallets/list
type WalletsListResponse struct {
	Status  string    `json:"status"`
	Wallets []Wallet  `json:"wallets"`
}

// APIError represents a failed API response
type APIError struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Client is the Nobitex REST API client for authenticated endpoints
type Client struct {
	token      string
	httpClient *http.Client
}

// NewClient creates a new Nobitex API client
func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// GetWallets fetches the list of all wallets with balances
// API: GET /users/wallets/list
// Rate limit: 20 requests per 2 minutes
func (c *Client) GetWallets() ([]Wallet, error) {
	req, err := http.NewRequest(http.MethodGet, APIBaseURL+"/users/wallets/list", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("User-Agent", UserAgent)

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
		var apiErr APIError
		if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Status == "failed" {
			return nil, fmt.Errorf("nobitex api error: %s - %s", apiErr.Code, apiErr.Message)
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result WalletsListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if result.Status != "ok" {
		return nil, fmt.Errorf("api returned status: %s", result.Status)
	}

	return result.Wallets, nil
}

// ParseBalance converts balance string to float64 (for Prometheus)
func ParseBalance(balanceStr string) float64 {
	f, _ := strconv.ParseFloat(balanceStr, 64)
	return f
}
