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
)

func init() {
	envFile, _ := godotenv.Read(".env")
	for key, value := range envFile {
		os.Setenv(key, value)
	}

	KUCOIN_API_KEY = os.Getenv("KUCOIN_API_KEY")
	KUCOIN_API_SECRET = os.Getenv("KUCOIN_API_SECRET")
	KUCOIN_API_PASSPHRASE = os.Getenv("KUCOIN_API_PASSPHRASE")
	KUCOIN_DEFAULT_SYMBOLS = os.Getenv("KUCOIN_DEFAULT_SYMBOLS")
	BINANCE_API_KEY = os.Getenv("BINANCE_API_KEY")
	BINANCE_API_SECRET = os.Getenv("BINANCE_API_SECRET")
	BINANCE_DEFAULT_SYMBOLS = os.Getenv("BINANCE_DEFAULT_SYMBOLS")
	PROXY_URI               = os.Getenv("PROXY_URI")
	NOBITEX_TOKEN           = os.Getenv("NOBITEX_TOKEN")
}
