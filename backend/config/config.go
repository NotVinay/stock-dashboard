package config

import (
	"bufio"
	"log"
	"os"
	"strings"
)

// Config represents the application configuration.
type Config struct {
	FinnhubAPIKey   string
	FinnhubAPIURL   string
	Port            string
	WSPort          string
	AllowedOrigins  []string
	Environment     string
}

var appConfig *Config

// GetAppConfig returns application Config.
func GetAppConfig() *Config {
	if appConfig == nil {
		loadConfig()
	}
	return appConfig
}

// loadConfig loads configuration from environment variables and .env file.
func loadConfig() *Config {
	// Load .env file if it exists.
	loadEnvFile()

	appConfig = &Config{
		FinnhubAPIKey:  getEnv("FINNHUB_API_KEY", ""),
		FinnhubAPIURL:  getEnv("FINNHUB_API_URL", "https://finnhub.io/api/v1"),
		Port:           getEnv("PORT", "8080"),
		WSPort:         getEnv("WS_PORT", "8081"),
		Environment:    getEnv("ENV", "development"),
	}

	// Parse allowed origins.
	origins := getEnv("ALLOWED_ORIGINS", "http://localhost:4200")
	appConfig.AllowedOrigins = strings.Split(origins, ",")

	// Validate required configuration.
	if appConfig.FinnhubAPIKey == "" {
		log.Fatal("FINNHUB_API_KEY is required. Please set it in your .env file or environment variables.")
	}

	log.Printf("Configuration loaded successfully. Environment: %s", appConfig.Environment)
	return appConfig
}

// loadEnvFile loads environment variables from .env file.
func loadEnvFile() {
	file, err := os.Open(".env")
	if err != nil {
		// .env file is optional.
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		
		// Don't override existing environment variables.
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

// getEnv gets an environment variable with a fallback value.
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}