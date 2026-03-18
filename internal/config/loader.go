package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var envVarRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// Load reads the YAML config file at path, interpolates ${ENV_VAR} placeholders,
// applies defaults, and validates required fields.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	// Interpolate ${ENV_VAR} patterns.
	expanded := envVarRe.ReplaceAllStringFunc(string(raw), func(match string) string {
		key := match[2 : len(match)-1]
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return match // leave as-is if not set
	})

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ReadTimeout.Duration == 0 {
		cfg.Server.ReadTimeout = Duration{30 * time.Second}
	}
	if cfg.Server.WriteTimeout.Duration == 0 {
		cfg.Server.WriteTimeout = Duration{30 * time.Second}
	}
	if cfg.Server.IdleTimeout.Duration == 0 {
		cfg.Server.IdleTimeout = Duration{120 * time.Second}
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "./flowgate.db"
	}
	if cfg.Database.MaxOpenConnections == 0 {
		cfg.Database.MaxOpenConnections = 5
	}
	if cfg.Database.MaxIdleConnections == 0 {
		cfg.Database.MaxIdleConnections = 2
	}
	if cfg.Transfer.WorkerPoolSize == 0 {
		cfg.Transfer.WorkerPoolSize = 10
	}
	if cfg.Transfer.QueueCapacity == 0 {
		cfg.Transfer.QueueCapacity = 1000
	}
	if cfg.Transfer.RetryAttempts == 0 {
		cfg.Transfer.RetryAttempts = 3
	}
	if cfg.Transfer.RetryBackoff.Duration == 0 {
		cfg.Transfer.RetryBackoff = Duration{5 * time.Second}
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
}

func validate(cfg *Config) error {
	var errs []string

	key := strings.TrimSpace(cfg.Security.SecretKey)
	if key == "" || strings.HasPrefix(key, "${") {
		errs = append(errs, "security.secret_key must be set (use the SECRET_KEY env var)")
	}

	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		errs = append(errs, "server.port must be between 1 and 65535")
	}

	if cfg.Transfer.WorkerPoolSize < 1 {
		errs = append(errs, "transfer.worker_pool_size must be at least 1")
	}

	if cfg.Transfer.QueueCapacity < 1 {
		errs = append(errs, "transfer.queue_capacity must be at least 1")
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
