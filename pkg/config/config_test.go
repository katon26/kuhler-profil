package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"kuhlerprofil/pkg/config"
	"kuhlerprofil/pkg/models"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.Default()
	if cfg.DefaultMode != models.ModeStandard {
		t.Errorf("DefaultMode = %v, want %v", cfg.DefaultMode, models.ModeStandard)
	}
	if cfg.DefaultBatteryLimit != 80 {
		t.Errorf("DefaultBatteryLimit = %d, want 80", cfg.DefaultBatteryLimit)
	}
	if cfg.PollIntervalMs != 1500 {
		t.Errorf("PollIntervalMs = %d, want 1500", cfg.PollIntervalMs)
	}
	if cfg.AutoCurve != false {
		t.Errorf("AutoCurve = %v, want false", cfg.AutoCurve)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "koolthing-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "sub", "koolthing.toml")

	initialCfg := models.Config{
		DefaultMode:         models.ModeBoost,
		DefaultBatteryLimit: 60,
		PollIntervalMs:      2000,
		AutoCurve:           true,
	}

	if err := config.Save(configPath, initialCfg); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}

	loadedCfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if loadedCfg.DefaultMode != models.ModeBoost {
		t.Errorf("loaded DefaultMode = %v, want %v", loadedCfg.DefaultMode, models.ModeBoost)
	}
	if loadedCfg.DefaultBatteryLimit != 60 {
		t.Errorf("loaded DefaultBatteryLimit = %d, want 60", loadedCfg.DefaultBatteryLimit)
	}
	if loadedCfg.PollIntervalMs != 2000 {
		t.Errorf("loaded PollIntervalMs = %d, want 2000", loadedCfg.PollIntervalMs)
	}
	if loadedCfg.AutoCurve != true {
		t.Errorf("loaded AutoCurve = %v, want true", loadedCfg.AutoCurve)
	}
}

func TestLoad_NonExistentFileReturnsDefault(t *testing.T) {
	nonExistentPath := "/tmp/does-not-exist-koolthing-cfg-12345/config.toml"
	cfg, err := config.Load(nonExistentPath)
	if err != nil {
		t.Fatalf("Load on non-existent file returned unexpected error: %v", err)
	}
	if cfg.DefaultMode != models.ModeStandard {
		t.Errorf("DefaultMode = %v, want %v", cfg.DefaultMode, models.ModeStandard)
	}
	if cfg.DefaultBatteryLimit != 80 {
		t.Errorf("DefaultBatteryLimit = %d, want 80", cfg.DefaultBatteryLimit)
	}
}

func TestLoad_CorruptedTOML(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "koolthing-corrupt-*.toml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString("invalid toml content = [[[ \n"); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	_, err = config.Load(tmpFile.Name())
	if err == nil {
		t.Errorf("Load on corrupted file expected error, got nil")
	}
}

func TestLoad_EmptyFieldsNormalized(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "koolthing-empty-*.toml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Empty TOML config
	if _, err := tmpFile.WriteString("# Empty config file\n"); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	cfg, err := config.Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load on empty config file error: %v", err)
	}
	if cfg.DefaultMode != models.ModeStandard {
		t.Errorf("DefaultMode = %v, want %v", cfg.DefaultMode, models.ModeStandard)
	}
	if cfg.DefaultBatteryLimit != 80 {
		t.Errorf("DefaultBatteryLimit = %d, want 80", cfg.DefaultBatteryLimit)
	}
	if cfg.PollIntervalMs != 1500 {
		t.Errorf("PollIntervalMs = %d, want 1500", cfg.PollIntervalMs)
	}
}

func TestDiscoverConfigPath(t *testing.T) {
	path := config.DiscoverConfigPath()
	if path == "" {
		t.Errorf("DiscoverConfigPath returned empty string")
	}

	// Test with custom XDG_CONFIG_HOME
	tmpDir, err := os.MkdirTemp("", "koolthing-xdg-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", origXDG)

	// Create user config file in custom XDG dir
	cfgDir := filepath.Join(tmpDir, config.DefaultUserConfigDir)
	_ = os.MkdirAll(cfgDir, 0755)
	cfgFile := filepath.Join(cfgDir, config.ConfigFileName)
	_ = os.WriteFile(cfgFile, []byte("default_mode = \"boost\"\n"), 0644)

	discovered := config.DiscoverConfigPath()
	if discovered != cfgFile && discovered != config.DefaultSystemConfigPath {
		t.Errorf("DiscoverConfigPath = %s, want %s or system path", discovered, cfgFile)
	}
}

func TestDiscoverConfigPath_LegacyFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "kuhlerprofil-xdg-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", origXDG)

	// Create legacy user config file only
	legacyDir := filepath.Join(tmpDir, config.LegacyUserConfigDir)
	_ = os.MkdirAll(legacyDir, 0755)
	legacyFile := filepath.Join(legacyDir, config.ConfigFileName)
	_ = os.WriteFile(legacyFile, []byte("default_mode = \"silent\"\n"), 0644)

	discovered := config.DiscoverConfigPath()
	if discovered != legacyFile && discovered != config.DefaultSystemConfigPath && discovered != config.LegacySystemConfigPath {
		t.Errorf("DiscoverConfigPath = %s, want legacy user path %s", discovered, legacyFile)
	}

	// Now create primary user config file as well - primary must take precedence over legacy
	primaryDir := filepath.Join(tmpDir, config.DefaultUserConfigDir)
	_ = os.MkdirAll(primaryDir, 0755)
	primaryFile := filepath.Join(primaryDir, config.ConfigFileName)
	_ = os.WriteFile(primaryFile, []byte("default_mode = \"boost\"\n"), 0644)

	discoveredPrimary := config.DiscoverConfigPath()
	if discoveredPrimary != primaryFile && discoveredPrimary != config.DefaultSystemConfigPath {
		t.Errorf("DiscoverConfigPath with both present = %s, want primary %s", discoveredPrimary, primaryFile)
	}
}

