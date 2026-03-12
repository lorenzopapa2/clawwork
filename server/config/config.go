package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port               string
	DBPath             string
	BSCRpcURL          string
	ContractAddr       string
	PlatformPrivateKey string
	USDTAddr           string
	ChainID            int64
	PlatformFeeBps     int
}

func Load() *Config {
	chainID, _ := strconv.ParseInt(getEnv("CHAIN_ID", "56"), 10, 64)

	return &Config{
		Port:               getEnv("PORT", "8080"),
		DBPath:             getEnv("DB_PATH", "agenthub.db"),
		BSCRpcURL:          getEnv("BSC_RPC_URL", "https://bsc-dataseed.binance.org/"),
		ContractAddr:       getEnv("CONTRACT_ADDR", ""),
		PlatformPrivateKey: getEnv("PLATFORM_PRIVATE_KEY", ""),
		USDTAddr:           getEnv("USDT_ADDR", "0x55d398326f99059fF775485246999027B3197955"),
		ChainID:            chainID,
		PlatformFeeBps:     200, // 2%
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
