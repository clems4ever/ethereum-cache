package config

import (
	"strconv"
	"strings"
)

type Config struct {
	Port              string  `mapstructure:"port"`
	UpstreamURL       string  `mapstructure:"upstream_url"`
	DatabaseDSN       string  `mapstructure:"database_dsn"`
	AuthToken         string  `mapstructure:"auth_token"`
	MaxCacheSize      string  `mapstructure:"max_cache_size_bytes"`
	CleanupSlackRatio float64 `mapstructure:"cleanup_slack_ratio"`
	RateLimit         float64 `mapstructure:"rate_limit"`
}

func (c *Config) GetMaxCacheSizeBytes() (int64, error) {
	return ParseBytes(c.MaxCacheSize)
}

func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	s = strings.ToUpper(s)

	var multiplier int64 = 1
	if strings.HasSuffix(s, "K") || strings.HasSuffix(s, "KB") {
		multiplier = 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "B"), "K")
	} else if strings.HasSuffix(s, "M") || strings.HasSuffix(s, "MB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "B"), "M")
	} else if strings.HasSuffix(s, "G") || strings.HasSuffix(s, "GB") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "B"), "G")
	}

	val, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, err
	}
	return val * multiplier, nil
}
