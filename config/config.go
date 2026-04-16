package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port               string
	DatabaseURL        string
	OpenAIAPIKey       string
	EmbeddingModel     string
	APIKey             string
	CrawlIntervalMin   int
	SyncIntervalMin    int
	SyncStalenessDays  int
	CrawlConcurrency   int
	CrawlDateFrom      time.Time
	CrawlDateTo        time.Time
	MaxArticlesPerBlog int
	CORSAllowedOrigins []string
	LogLevel           string
}

func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required")
	}
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("API_KEY is required")
	}

	crawlDateFrom, err := parseDate("CRAWL_DATE_FROM", "2010-01-01")
	if err != nil {
		return nil, err
	}
	defaultDateTo := time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	crawlDateTo, err := parseDate("CRAWL_DATE_TO", defaultDateTo)
	if err != nil {
		return nil, err
	}

	corsOrigins := []string{"*"}
	if s := os.Getenv("CORS_ALLOWED_ORIGINS"); s != "" {
		corsOrigins = strings.Split(s, ",")
	}

	return &Config{
		Port:               getEnvOrDefault("PORT", "8080"),
		DatabaseURL:        dbURL,
		OpenAIAPIKey:       openaiKey,
		EmbeddingModel:     getEnvOrDefault("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
		APIKey:             apiKey,
		CrawlIntervalMin:   getEnvInt("CRAWL_INTERVAL_MIN", 360),
		SyncIntervalMin:    getEnvInt("SYNC_INTERVAL_MIN", 60),
		SyncStalenessDays:  getEnvInt("SYNC_STALENESS_DAYS", 30),
		CrawlConcurrency:   getEnvInt("CRAWL_CONCURRENCY", 5),
		CrawlDateFrom:      crawlDateFrom,
		CrawlDateTo:        crawlDateTo,
		MaxArticlesPerBlog: getEnvInt("MAX_ARTICLES_PER_BLOG", 5),
		CORSAllowedOrigins: corsOrigins,
		LogLevel:           getEnvOrDefault("LOG_LEVEL", "info"),
	}, nil
}

func parseDate(envKey, defaultVal string) (time.Time, error) {
	s := getEnvOrDefault(envKey, defaultVal)
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: invalid date %q: %w", envKey, s, err)
	}
	return t, nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
