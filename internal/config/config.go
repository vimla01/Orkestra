package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port int `yaml:"port"`
}

// HealthConfig holds health-check polling configuration.
type HealthConfig struct {
	PollIntervalSeconds int `yaml:"pollIntervalSeconds"`
}

// FailoverConfig controls moving deployments off unhealthy clusters.
type FailoverConfig struct {
	// Enabled turns on the failover controller. It is off unless set.
	Enabled bool `yaml:"enabled"`

	// GracePeriodSeconds is how long a cluster must stay Unhealthy before
	// its deployments are moved elsewhere.
	GracePeriodSeconds int `yaml:"gracePeriodSeconds"`
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level string `yaml:"level"`
}

// Config is the top-level configuration for Orkestra.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Health   HealthConfig   `yaml:"health"`
	Failover FailoverConfig `yaml:"failover"`
	Log      LogConfig      `yaml:"log"`
}

// Load reads a YAML configuration file from path and returns a Config
// with defaults applied for any unset fields.
//
// Defaults:
//   - Server.Port: 8080
//   - Health.PollIntervalSeconds: 30
//   - Failover.GracePeriodSeconds: 60
//   - Log.Level: "info"
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Apply defaults for zero-value fields.
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Health.PollIntervalSeconds <= 0 {
		cfg.Health.PollIntervalSeconds = 30
	}
	if cfg.Failover.GracePeriodSeconds <= 0 {
		cfg.Failover.GracePeriodSeconds = 60
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}

	return cfg, nil
}
