package config

import (
	"fmt"
	"time"
)

// Duration is a time.Duration that unmarshals from YAML duration strings (e.g. "30s").
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		// Fallback: try raw nanosecond int.
		var n int64
		if err2 := unmarshal(&n); err2 != nil {
			return err
		}
		d.Duration = time.Duration(n)
		return nil
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

// Config is the root configuration structure.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Transfer  TransferConfig  `yaml:"transfer"`
	Logging   LoggingConfig   `yaml:"logging"`
	Security  SecurityConfig  `yaml:"security"`
	Dashboard DashboardConfig `yaml:"dashboard"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host         string   `yaml:"host"`
	Port         int      `yaml:"port"`
	ReadTimeout  Duration `yaml:"read_timeout"`
	WriteTimeout Duration `yaml:"write_timeout"`
	IdleTimeout  Duration `yaml:"idle_timeout"`
}

// DatabaseConfig holds SQLite settings.
type DatabaseConfig struct {
	Path               string `yaml:"path"`
	MaxOpenConnections int    `yaml:"max_open_connections"`
	MaxIdleConnections int    `yaml:"max_idle_connections"`
}

// TransferConfig holds worker pool settings.
type TransferConfig struct {
	WorkerPoolSize int      `yaml:"worker_pool_size"`
	QueueCapacity  int      `yaml:"queue_capacity"`
	RetryAttempts  int      `yaml:"retry_attempts"`
	RetryBackoff   Duration `yaml:"retry_backoff"`
}

// LoggingConfig holds slog settings.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// SecurityConfig holds the AES-GCM master key.
type SecurityConfig struct {
	SecretKey string `yaml:"secret_key"`
}

// DashboardConfig holds dashboard UI settings.
type DashboardConfig struct {
	Enabled     bool   `yaml:"enabled"`
	AuthEnabled bool   `yaml:"auth_enabled"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
}
