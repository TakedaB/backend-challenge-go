package postgres

// Package postgres wires the application to a real PostgreSQL
// database: connection pool setup and the repositories that translate
// domain aggregates to and from SQL rows.

import (
	"fmt"
	"os"
)

// Config holds the connection parameters read from environment
// variables (see .env.example). No default password or user is
// hardcoded here — every value must come from the environment.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// NewConfigFromEnv reads DB_HOST, DB_PORT, DB_USER, DB_PASSWORD,
// DB_NAME and DB_SSLMODE from the environment.
func NewConfigFromEnv() Config {
	return Config{
		Host:     getEnvOrDefault("DB_HOST", "localhost"),
		Port:     getEnvOrDefault("DB_PORT", "5432"),
		User:     getEnvOrDefault("DB_USER", "app"),
		Password: getEnvOrDefault("DB_PASSWORD", "app"),
		DBName:   getEnvOrDefault("DB_NAME", "backend_challenge"),
		SSLMode:  getEnvOrDefault("DB_SSLMODE", "disable"),
	}
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// DSN renders the config as a PostgreSQL connection string.
func (c Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}
