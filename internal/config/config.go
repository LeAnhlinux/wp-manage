package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server       ServerConfig
	Database     DatabaseConfig
	Redis        RedisConfig
	JWT          JWTConfig
	Agent        AgentConfig
	Panel        PanelConfig
	Google       GoogleConfig
	FCMServerKey string // Firebase Cloud Messaging legacy server key (FCM_SERVER_KEY)
	FCMProjectID string // GCP project ID for FCM v1 API (FCM_PROJECT_ID)
}

type GoogleConfig struct {
	PageSpeedAPIKey string
}

type ServerConfig struct {
	Host string
	Port int
	Mode string // development, production
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
}

type JWTConfig struct {
	Secret     string
	Expiration time.Duration
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

type AgentConfig struct {
	GRPCPort       int
	HeartbeatSec   int
	UpdateCheckSec int
}

type PanelConfig struct {
	Domain    string
	InstallURL string
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		d.User, d.Password, d.Host, d.Port, d.Name)
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8080),
			Mode: getEnv("SERVER_MODE", "development"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "127.0.0.1"),
			Port:     getEnvInt("DB_PORT", 3306),
			User:     getEnv("DB_USER", "wp_manage"),
			Password: getEnv("DB_PASSWORD", ""),
			Name:     getEnv("DB_NAME", "wp_manage"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "127.0.0.1"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: JWTConfig{
			Secret:     getEnv("JWT_SECRET", "change-me-in-production"),
			Expiration: time.Duration(getEnvInt("JWT_EXPIRATION_HOURS", 24)) * time.Hour,
		},
		Agent: AgentConfig{
			GRPCPort:       getEnvInt("AGENT_GRPC_PORT", 50051),
			HeartbeatSec:   getEnvInt("AGENT_HEARTBEAT_SEC", 30),
			UpdateCheckSec: getEnvInt("AGENT_UPDATE_CHECK_SEC", 300),
		},
		Panel: PanelConfig{
			Domain:    getEnv("PANEL_DOMAIN", "panel.wpm.vn"),
			InstallURL: getEnv("INSTALL_URL", "https://install.wpm.vn"),
		},
		Google: GoogleConfig{
			PageSpeedAPIKey: getEnv("GOOGLE_PAGESPEED_API_KEY", ""),
		},
		FCMServerKey: getEnv("FCM_SERVER_KEY", ""),
		FCMProjectID: getEnv("FCM_PROJECT_ID", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
