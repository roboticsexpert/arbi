package config

import (
	"os"

	"github.com/joho/godotenv"
)

var (
	KUCOIN_API_KEY          string
	KUCOIN_API_SECRET       string
	KUCOIN_API_PASSPHRASE   string
	KUCOIN_DEFAULT_SYMBOLS  string
	BINANCE_API_KEY         string
	BINANCE_API_SECRET      string
	BINANCE_DEFAULT_SYMBOLS string
	PROXY_URI               string
	NOBITEX_TOKEN           string
	NOBITEX_DEFAULT_SYMBOLS string
	ECOGOLD_TOKEN           string
	ECOGOLD_PASSWORD        string
	DASHBOARD_TOKEN         string
	DASHBOARD_ORIGINS       string
	MT5_INGEST_TOKEN        string
	MT5_STALE_SECONDS       string
	HISTORY_DB_PATH         string
	HISTORY_RETENTION_DAYS  string
)

func init() {
	// Load .env as defaults only: real environment variables (e.g. those injected
	// by the platform) always win over the baked-in file.
	envFile, _ := godotenv.Read(".env")
	for key, value := range envFile {
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}

	KUCOIN_API_KEY = os.Getenv("KUCOIN_API_KEY")
	KUCOIN_API_SECRET = os.Getenv("KUCOIN_API_SECRET")
	KUCOIN_API_PASSPHRASE = os.Getenv("KUCOIN_API_PASSPHRASE")
	KUCOIN_DEFAULT_SYMBOLS = os.Getenv("KUCOIN_DEFAULT_SYMBOLS")
	BINANCE_API_KEY = os.Getenv("BINANCE_API_KEY")
	BINANCE_API_SECRET = os.Getenv("BINANCE_API_SECRET")
	BINANCE_DEFAULT_SYMBOLS = os.Getenv("BINANCE_DEFAULT_SYMBOLS")
	PROXY_URI = os.Getenv("PROXY_URI")
	NOBITEX_TOKEN = os.Getenv("NOBITEX_TOKEN")
	NOBITEX_DEFAULT_SYMBOLS = os.Getenv("NOBITEX_DEFAULT_SYMBOLS")
	ECOGOLD_TOKEN = os.Getenv("ECOGOLD_TOKEN")
	ECOGOLD_PASSWORD = os.Getenv("ECOGOLD_PASSWORD")
	DASHBOARD_TOKEN = os.Getenv("DASHBOARD_TOKEN")
	DASHBOARD_ORIGINS = os.Getenv("DASHBOARD_ORIGINS")
	MT5_INGEST_TOKEN = os.Getenv("MT5_INGEST_TOKEN")
	MT5_STALE_SECONDS = os.Getenv("MT5_STALE_SECONDS")
	HISTORY_DB_PATH = os.Getenv("HISTORY_DB_PATH")
	HISTORY_RETENTION_DAYS = os.Getenv("HISTORY_RETENTION_DAYS")
}
