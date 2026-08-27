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
	BlacklistTTL   time.Duration // int → time.Duration으로 변경
	WhitelistIPs   map[string]bool
	AITimeout      time.Duration

	// ─── AI gRPC 연동 ───────────────────────────────
	UseMockAI  bool   // true면 Mock, false면 실제 gRPC 서버 호출
	AIGrpcAddr string // AI 이상탐지 gRPC 서버 주소 (host:port)
	AIGrpcTLS  bool   // AI gRPC 연결에 TLS 사용 여부

	// ─── 서비스앱(백엔드) HTTPS 연동 ─────────────────
	TargetInsecureSkipVerify bool // self-signed 인증서 등 TLS 검증 skip
	PreserveHostHeader       bool // true면 원본 Host 유지, false면 타깃 Host로 재작성
}

func Load() *Config {
	godotenv.Load()

	whitelistRaw := strings.Split(getEnv("WHITELIST_IPS", ""), ",")
	whitelist := make(map[string]bool)
	for _, ip := range whitelistRaw {
		trimmed := strings.TrimSpace(ip)
		if trimmed != "" {
			whitelist[trimmed] = true
		}
	}

	blockThreshold, _ := strconv.ParseFloat(getEnv("BLOCK_THRESHOLD", "0.8"), 64)
	blacklistTTL, _ := strconv.Atoi(getEnv("BLACKLIST_TTL", "3600"))
	aiTimeout, _ := strconv.Atoi(getEnv("AI_TIMEOUT", "5"))

	return &Config{
		TargetURL:      getEnv("TARGET_URL", "http://localhost:8080"),
		RedisAddr:      getEnv("REDIS_ADDR", "localhost:6379"),
		Port:           getEnv("PORT", "9000"),
		BlockThreshold: blockThreshold,
		BlacklistTTL:   time.Duration(blacklistTTL) * time.Second, // 변환
		WhitelistIPs:   whitelist,
		AITimeout:      time.Duration(aiTimeout) * time.Second,

		UseMockAI:  getEnvBool("USE_MOCK_AI", false),
		AIGrpcAddr: getEnv("AI_GRPC_ADDR", "localhost:50051"),
		AIGrpcTLS:  getEnvBool("AI_GRPC_TLS", false),

		TargetInsecureSkipVerify: getEnvBool("TARGET_INSECURE_SKIP_VERIFY", false),
		PreserveHostHeader:       getEnvBool("PRESERVE_HOST_HEADER", false),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// "true"/"1"/"yes"/"on" → true (대소문자 무시)
func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return defaultVal
	}
}
