package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	AWS       AWSConfig       `toml:"aws"`
	Detection DetectionConfig `toml:"detection"`
	Output    OutputConfig    `toml:"output"`
}

type AWSConfig struct {
	Profile   string `toml:"profile"`
	Region    string `toml:"region"`
	AccountID string `toml:"account_id"`
}

type DetectionConfig struct {
	ThresholdSigma float64 `toml:"threshold_sigma"`
	BaselineDays   int     `toml:"baseline_days"`
	MinDeltaUSD    float64 `toml:"min_delta_usd"`
}

type OutputConfig struct {
	Format string `toml:"format"`
}

func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return &cfg, nil
}

func Defaults() *Config {
	return &Config{
		AWS: AWSConfig{
			Profile: "default",
			Region:  "us-east-1",
		},
		Detection: DetectionConfig{
			ThresholdSigma: 2.0,
			BaselineDays:   7,
			MinDeltaUSD:    50,
		},
		Output: OutputConfig{Format: "text"},
	}
}

func Write(path string, cfg *Config) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
