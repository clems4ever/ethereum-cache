package config

type Config struct {
	Port              string  `mapstructure:"port"`
	UpstreamURL       string  `mapstructure:"upstream_url"`
	DatabaseDSN       string  `mapstructure:"database_dsn"`
	AuthToken         string  `mapstructure:"auth_token"`
	MaxCacheSizeBytes int64   `mapstructure:"max_cache_size_bytes"`
	CleanupSlackRatio float64 `mapstructure:"cleanup_slack_ratio"`
	RateLimit         float64 `mapstructure:"rate_limit"`
}
