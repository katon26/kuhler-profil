package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"kuhlerprofil/pkg/config"
	"kuhlerprofil/pkg/driver"
	"kuhlerprofil/pkg/models"
)

const (
	// AutoCurveHighTemp is the CPU temperature threshold (°C) above which ModeBoost is activated.
	AutoCurveHighTemp = 75.0
	// AutoCurveLowTemp is the CPU temperature threshold (°C) below which the governor returns to ModeStandard.
	AutoCurveLowTemp = 55.0
	// DefaultTelemetryChannelBuffer is the buffer capacity for telemetry subscriber channels.
	DefaultTelemetryChannelBuffer = 16
)

// Engine is the central state machine and telemetry coordinator for KoolThing.
// It supervises hardware drivers, applies thermal mode hysteresis, enforces battery limits,
// and broadcasts real-time telemetry to connected consumers (D-Bus, CLI, TUI).
type Engine struct {
	mu          sync.RWMutex
	driver      driver.HardwareDriver
	cfg         models.Config
	configPath  string
	telemetry   models.Telemetry
	autoMode    bool
	subscribers map[chan models.Telemetry]struct{}
	subMu       sync.Mutex
}

// NewEngine instantiates a new Engine with hardware driver and initial configuration.
func NewEngine(drv driver.HardwareDriver, cfg models.Config, configPath string) *Engine {
	if cfg.PollIntervalMs <= 0 {
		cfg.PollIntervalMs = 1500
	}
	if cfg.DefaultMode == "" {
		cfg.DefaultMode = models.ModeStandard
	}
	if cfg.DefaultBatteryLimit <= 0 {
		cfg.DefaultBatteryLimit = 80
	}

	eng := &Engine{
		driver:      drv,
		cfg:         cfg,
		configPath:  configPath,
		autoMode:    cfg.AutoCurve,
		subscribers: make(map[chan models.Telemetry]struct{}),
		telemetry: models.Telemetry{
			ActiveMode:   cfg.DefaultMode,
			BatteryLimit: cfg.DefaultBatteryLimit,
			AutoMode:     cfg.AutoCurve,
		},
	}

	// Probe and initialize telemetry state from hardware if accessible
	if initialTelem, err := drv.ReadTelemetry(); err == nil {
		if initialTelem.ActiveMode != "" {
			eng.telemetry.ActiveMode = initialTelem.ActiveMode
		}
		if initialTelem.BatteryLimit > 0 {
			eng.telemetry.BatteryLimit = initialTelem.BatteryLimit
		}
		eng.telemetry.CPUTemp = initialTelem.CPUTemp
		eng.telemetry.Fan1RPM = initialTelem.Fan1RPM
		eng.telemetry.Fan2RPM = initialTelem.Fan2RPM
		eng.telemetry.BatteryPercent = initialTelem.BatteryPercent
		eng.telemetry.OnAC = initialTelem.OnAC
	}

	return eng
}

// Start launches the background polling, telemetry update, and auto-governor loop.
// It runs until the provided context is canceled.
func (e *Engine) Start(ctx context.Context) {
	interval := time.Duration(e.cfg.PollIntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = 1500 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial poll immediately
	e.pollAndGovern()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.pollAndGovern()
		}
	}
}

// pollAndGovern reads telemetry from hardware, applies thermal curve hysteresis if AutoMode is active,
// and broadcasts updated metrics to subscribers.
func (e *Engine) pollAndGovern() {
	var telemSnapshot models.Telemetry

	e.mu.Lock()
	rawTelem, err := e.driver.ReadTelemetry()
	if err == nil {
		e.telemetry.CPUTemp = rawTelem.CPUTemp
		e.telemetry.Fan1RPM = rawTelem.Fan1RPM
		e.telemetry.Fan2RPM = rawTelem.Fan2RPM
		e.telemetry.BatteryPercent = rawTelem.BatteryPercent
		e.telemetry.OnAC = rawTelem.OnAC
		if rawTelem.BatteryLimit > 0 {
			e.telemetry.BatteryLimit = rawTelem.BatteryLimit
		}
		if rawTelem.ActiveMode != "" && !e.autoMode {
			e.telemetry.ActiveMode = rawTelem.ActiveMode
		}
	}

	// Auto-Governor Thermal Hysteresis Logic
	if e.autoMode {
		currentMode := e.telemetry.ActiveMode
		temp := e.telemetry.CPUTemp

		if temp >= AutoCurveHighTemp && currentMode != models.ModeBoost {
			// Trigger Boost mode at elevated thermal thresholds
			if err := e.driver.SetThermalMode(models.ModeBoost); err == nil {
				e.telemetry.ActiveMode = models.ModeBoost
			}
		} else if temp <= AutoCurveLowTemp && currentMode == models.ModeBoost {
			// Return to Standard mode when temperature sufficiently cools down
			targetMode := e.cfg.DefaultMode
			if targetMode == models.ModeBoost {
				targetMode = models.ModeStandard
			}
			if err := e.driver.SetThermalMode(targetMode); err == nil {
				e.telemetry.ActiveMode = targetMode
			}
		}
		// In the band between LowTemp and HighTemp, maintain current active state (hysteresis)
	}

	e.telemetry.AutoMode = e.autoMode
	telemSnapshot = e.telemetry
	e.mu.Unlock()

	// Broadcast telemetry to subscribers
	e.broadcastTelemetry(telemSnapshot)
}

// broadcastTelemetry dispatches a telemetry snapshot to all active subscriber channels non-blockingly.
func (e *Engine) broadcastTelemetry(telem models.Telemetry) {
	e.subMu.Lock()
	defer e.subMu.Unlock()

	for ch := range e.subscribers {
		select {
		case ch <- telem:
		default:
			// Buffer full: drain stale entry and enqueue newest telemetry
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- telem:
			default:
			}
		}
	}
}

// SetMode configures the active thermal profile mode and updates persistent configuration.
func (e *Engine) SetMode(mode models.ThermalMode) error {
	switch mode {
	case models.ModeStandard, models.ModeBoost, models.ModeSilent:
	default:
		return fmt.Errorf("unsupported thermal mode: %q (valid: standard, boost, silent)", mode)
	}

	e.mu.Lock()
	if err := e.driver.SetThermalMode(mode); err != nil {
		e.mu.Unlock()
		return fmt.Errorf("failed to set thermal mode on hardware: %w", err)
	}

	e.telemetry.ActiveMode = mode
	e.cfg.DefaultMode = mode
	snapshot := e.telemetry
	cfgCopy := e.cfg
	configPath := e.configPath
	e.mu.Unlock()

	if configPath != "" {
		_ = config.Save(configPath, cfgCopy)
	}

	e.broadcastTelemetry(snapshot)
	return nil
}

// SetBatteryLimit configures the battery charging threshold and updates persistent configuration.
func (e *Engine) SetBatteryLimit(limit int32) error {
	if err := models.ValidateBatteryLimit(limit); err != nil {
		return err
	}

	e.mu.Lock()
	if err := e.driver.SetBatteryLimit(limit); err != nil {
		e.mu.Unlock()
		return fmt.Errorf("failed to set battery limit on hardware: %w", err)
	}

	e.telemetry.BatteryLimit = limit
	e.cfg.DefaultBatteryLimit = limit
	snapshot := e.telemetry
	cfgCopy := e.cfg
	configPath := e.configPath
	e.mu.Unlock()

	if configPath != "" {
		_ = config.Save(configPath, cfgCopy)
	}

	e.broadcastTelemetry(snapshot)
	return nil
}

// SetAutoMode toggles the automatic thermal curve governor.
func (e *Engine) SetAutoMode(enabled bool) error {
	e.mu.Lock()
	e.autoMode = enabled
	e.telemetry.AutoMode = enabled
	e.cfg.AutoCurve = enabled
	snapshot := e.telemetry
	cfgCopy := e.cfg
	configPath := e.configPath
	e.mu.Unlock()

	if configPath != "" {
		_ = config.Save(configPath, cfgCopy)
	}

	e.broadcastTelemetry(snapshot)
	return nil
}

// GetTelemetry returns a thread-safe snapshot of the latest hardware telemetry.
func (e *Engine) GetTelemetry() models.Telemetry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.telemetry
}

// GetConfig returns a thread-safe snapshot of current daemon configuration.
func (e *Engine) GetConfig() models.Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg
}

// GetDriver returns the underlying HardwareDriver instance.
func (e *Engine) GetDriver() driver.HardwareDriver {
	return e.driver
}

// SubscribeTelemetry creates and registers a buffered subscription channel for real-time telemetry updates.
func (e *Engine) SubscribeTelemetry() <-chan models.Telemetry {
	ch := make(chan models.Telemetry, DefaultTelemetryChannelBuffer)

	// Send immediate snapshot if available
	snapshot := e.GetTelemetry()
	ch <- snapshot

	e.subMu.Lock()
	e.subscribers[ch] = struct{}{}
	e.subMu.Unlock()

	return ch
}

// UnsubscribeTelemetry removes and closes a previously registered telemetry subscription channel.
func (e *Engine) UnsubscribeTelemetry(ch <-chan models.Telemetry) {
	if ch == nil {
		return
	}

	e.subMu.Lock()
	defer e.subMu.Unlock()

	for sub := range e.subscribers {
		if (<-chan models.Telemetry)(sub) == ch {
			delete(e.subscribers, sub)
			close(sub)
			break
		}
	}
}
