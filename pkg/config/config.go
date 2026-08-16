package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"koolthing/pkg/models"
)

const (
	// DefaultSystemConfigPath is the primary system-wide configuration path.
	DefaultSystemConfigPath = "/etc/koolthing/config.toml"
	// DefaultUserConfigDir is the fallback user config folder under ~/.config.
	DefaultUserConfigDir = "koolthing"
	// ConfigFileName is the standard configuration file name.
	ConfigFileName = "config.toml"
)

// Default returns safe default daemon configuration.
func Default() models.Config {
	return models.DefaultConfig()
}

// DiscoverConfigPath determines the most appropriate configuration file path.
// It checks for an existing system configuration at /etc/koolthing/config.toml first.
// If not found, it checks the user's XDG config directory (~/.config/koolthing/config.toml).
// If neither exists, it returns the system path if running as root (UID 0), or user path otherwise.
func DiscoverConfigPath() string {
	// 1. If system config exists, prefer it
	if _, err := os.Stat(DefaultSystemConfigPath); err == nil {
		return DefaultSystemConfigPath
	}

	// 2. Build user config path
	userPath := getUserConfigPath()

	// If user config exists, return it
	if userPath != "" {
		if _, err := os.Stat(userPath); err == nil {
			return userPath
		}
	}

	// 3. Fallback based on privilege
	if os.Geteuid() == 0 {
		return DefaultSystemConfigPath
	}

	if userPath != "" {
		return userPath
	}

	return DefaultSystemConfigPath
}

func getUserConfigPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, DefaultUserConfigDir, ConfigFileName)
}

// Load reads and parses a TOML configuration file.
// If the target file does not exist, it returns the default configuration without error.
func Load(path string) (models.Config, error) {
	if path == "" {
		path = DiscoverConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Default(), fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	cfg := Default()
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("failed to parse TOML config at %q: %w", path, err)
	}

	// Normalize / Sanitize defaults if parsed fields were empty
	if cfg.DefaultMode == "" {
		cfg.DefaultMode = models.ModeStandard
	}
	if cfg.DefaultBatteryLimit <= 0 {
		cfg.DefaultBatteryLimit = 80
	}
	if cfg.PollIntervalMs <= 0 {
		cfg.PollIntervalMs = 1500
	}

	return cfg, nil
}

// Save marshals and writes a Config structure to disk in TOML format.
// It automatically creates any missing parent directories.
func Save(path string, cfg models.Config) error {
	if path == "" {
		path = DiscoverConfigPath()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory %q: %w", dir, err)
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config to TOML: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config to %q: %w", path, err)
	}

	return nil
}
