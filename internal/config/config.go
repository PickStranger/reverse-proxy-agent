package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	TargetURL      string
	RedisAddr      string
	Port           string
	BlockThreshold float64
	BlacklistTTL   int
	WhitelistIPs   map[string]bool
	AITimeout      time.Duration
}

func Load() *Config {
	godotenv.Load()

	whitelistRaw := strings.Split(getEnv("WHITELIST_IPS", ""), ",")
	whitelist := make(map[string]bool)
	for _, ip := range whitelistRaw {
		whitelist[strings.TrimSpace(ip)] = true
	}

	blockThreshold, _ := strconv.ParseFloat(getEnv("BLOCK_THRESHOLD", "0.8"), 64)
	blacklistTTL, _ := strconv.Atoi(getEnv("BLACKLIST_TTL", "3600"))
	aiTimeout, _ := strconv.Atoi(getEnv("AI_TIMEOUT", "5"))

	return &Config{
		TargetURL:      getEnv("TARGET_URL", "http://localhost:8080"),
		RedisAddr:      getEnv("REDIS_ADDR", "localhost:6379"),
		Port:           getEnv("PORT", "9000"),
		BlockThreshold: blockThreshold,
		BlacklistTTL:   blacklistTTL,
		WhitelistIPs:   whitelist,
		AITimeout:      time.Duration(aiTimeout) * time.Second,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
