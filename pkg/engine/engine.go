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
	mu             sync.RWMutex
	driver         driver.HardwareDriver
	cfg            models.Config
	configPath     string
	telemetry      models.Telemetry
	autoMode       bool
	subscribers    map[chan models.Telemetry]struct{}
	subMu          sync.Mutex
	pendingTarget  models.ThermalMode
	pendingSince   time.Time
	lastModeChange time.Time
	dwellDuration  time.Duration
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
	if cfg.ActiveCurveProfile == "" {
		cfg.ActiveCurveProfile = "balanced"
	}
	if len(cfg.Curves) == 0 {
		cfg.Curves = models.DefaultCurveProfiles()
	} else {
		for k, v := range models.DefaultCurveProfiles() {
			if _, exists := cfg.Curves[k]; !exists {
				cfg.Curves[k] = v
			}
		}
	}

	dwell := 3 * time.Second
	pollInterval := time.Duration(cfg.PollIntervalMs) * time.Millisecond
	if pollInterval > 0 && pollInterval < 500*time.Millisecond {
		dwell = 2 * pollInterval
	}

	eng := &Engine{
		driver:        drv,
		cfg:           cfg,
		configPath:    configPath,
		autoMode:      cfg.AutoCurve,
		dwellDuration: dwell,
		subscribers:   make(map[chan models.Telemetry]struct{}),
		telemetry: models.Telemetry{
			ActiveMode:         cfg.DefaultMode,
			BatteryLimit:       cfg.DefaultBatteryLimit,
			AutoMode:           cfg.AutoCurve,
			ActiveCurveProfile: cfg.ActiveCurveProfile,
			HasHardwareCurve:   drv.GetHardwareCurveCaps().Supported,
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
		if initialTelem.HasHardwareCurve {
			eng.telemetry.HasHardwareCurve = true
		}
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
	e.telemetry.HasHardwareCurve = e.driver.GetHardwareCurveCaps().Supported
	e.telemetry.ActiveCurveProfile = e.cfg.ActiveCurveProfile

	// Auto-Governor Thermal Hysteresis Logic
	if e.autoMode {
		if caps := e.driver.GetHardwareCurveCaps(); caps.Supported {
			fanCount := caps.FanCount
			if fanCount <= 0 {
				fanCount = 1
			}
			for f := 1; f <= fanCount; f++ {
				if enabled, err := e.driver.IsHardwareCurveEnabled(f); err == nil && !enabled {
					_ = e.driver.SetHardwareCurveEnabled(f, 1)
				}
			}
		} else {
			currentMode := e.telemetry.ActiveMode
			temp := e.telemetry.CPUTemp

			// Evaluate active curve profile
			profName := e.cfg.ActiveCurveProfile
			profile, ok := e.cfg.Curves[profName]
			if !ok {
				defaults := models.DefaultCurveProfiles()
				if defProf, defOk := defaults[profName]; defOk {
					profile = defProf
				} else {
					profile = defaults["balanced"]
				}
			}

			targetMode := evaluateCurveProfile(profile, temp, currentMode, e.cfg.DefaultMode)
			now := time.Now()

			if targetMode != "" && targetMode != currentMode {
				currRank := modeRank(currentMode)
				targRank := modeRank(targetMode)

				if targRank > currRank {
					// Immediate UP-step for cooling safety
					e.pendingTarget = ""
					if err := e.driver.SetThermalMode(targetMode); err == nil {
						e.telemetry.ActiveMode = targetMode
						e.lastModeChange = now
					}
				} else {
					// DOWN-step: require anti-flutter dwell
					dwell := e.dwellDuration
					if dwell <= 0 {
						dwell = 3 * time.Second
					}
					if e.pendingTarget != targetMode {
						e.pendingTarget = targetMode
						e.pendingSince = now
					} else if now.Sub(e.pendingSince) >= dwell {
						if err := e.driver.SetThermalMode(targetMode); err == nil {
							e.telemetry.ActiveMode = targetMode
							e.lastModeChange = now
							e.pendingTarget = ""
						}
					}
				}
			} else {
				e.pendingTarget = ""
			}
		}
	} else {
		e.pendingTarget = ""
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
	e.pendingTarget = ""
	e.lastModeChange = time.Now()
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
	e.pendingTarget = ""

	// If hardware ACPI custom fan curve is supported, enable or disable hardware curve on all fans
	if caps := e.driver.GetHardwareCurveCaps(); caps.Supported {
		fanCount := caps.FanCount
		if fanCount <= 0 {
			fanCount = 1
		}
		modeVal := 2
		if enabled {
			modeVal = 1
		}
		for f := 1; f <= fanCount; f++ {
			_ = e.driver.SetHardwareCurveEnabled(f, modeVal)
		}
	} else if enabled {
		// Software governor: immediately evaluate target mode and apply it to hardware
		profName := e.cfg.ActiveCurveProfile
		profile, ok := e.cfg.Curves[profName]
		if !ok {
			defaults := models.DefaultCurveProfiles()
			if defProf, defOk := defaults[profName]; defOk {
				profile = defProf
			} else {
				profile = defaults["balanced"]
			}
		}
		targetMode := evaluateCurveProfile(profile, e.telemetry.CPUTemp, "", e.cfg.DefaultMode)
		if targetMode != "" {
			if err := e.driver.SetThermalMode(targetMode); err == nil {
				e.telemetry.ActiveMode = targetMode
				e.lastModeChange = time.Now()
			}
		}
	}

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

// SetDwellDuration overrides the anti-flutter down-step hold duration.
func (e *Engine) SetDwellDuration(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dwellDuration = d
}

// GetActiveCurveProfile returns the active curve profile name.
func (e *Engine) GetActiveCurveProfile() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.cfg.ActiveCurveProfile == "" {
		return "balanced"
	}
	return e.cfg.ActiveCurveProfile
}

// GetCurveProfiles returns all configured and default curve profiles.
func (e *Engine) GetCurveProfiles() map[string]models.CurveProfile {
	e.mu.RLock()
	defer e.mu.RUnlock()

	res := make(map[string]models.CurveProfile)
	for k, v := range models.DefaultCurveProfiles() {
		res[k] = v
	}
	for k, v := range e.cfg.Curves {
		res[k] = v
	}
	return res
}

// SetCurveProfile activates a curve profile by name and persists it.
func (e *Engine) SetCurveProfile(profileName string) error {
	e.mu.Lock()
	profiles := make(map[string]models.CurveProfile)
	for k, v := range models.DefaultCurveProfiles() {
		profiles[k] = v
	}
	for k, v := range e.cfg.Curves {
		profiles[k] = v
	}

	profile, ok := profiles[profileName]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("unknown curve profile %q", profileName)
	}

	e.cfg.ActiveCurveProfile = profileName
	e.telemetry.ActiveCurveProfile = profileName
	e.pendingTarget = ""

	// If hardware ACPI curve is supported, write points to hardware sysfs for all fans
	if caps := e.driver.GetHardwareCurveCaps(); caps.Supported {
		fanCount := caps.FanCount
		if fanCount <= 0 {
			fanCount = 1
		}
		for f := 1; f <= fanCount; f++ {
			_ = e.driver.WriteHardwareCurve(f, profile.Points)
			if e.autoMode {
				_ = e.driver.SetHardwareCurveEnabled(f, 1)
			}
		}
	} else if e.autoMode {
		// Software governor path: immediately evaluate target mode for the newly chosen profile
		// using clean initial evaluation so switching to "quiet" at 60°C immediately activates Silent mode!
		targetMode := evaluateCurveProfile(profile, e.telemetry.CPUTemp, "", e.cfg.DefaultMode)
		if targetMode != "" {
			if err := e.driver.SetThermalMode(targetMode); err == nil {
				e.telemetry.ActiveMode = targetMode
				e.lastModeChange = time.Now()
			}
		}
	}

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

// modeRank assigns an ordinal integer to thermal modes for directional transition logic (up-step vs down-step).
func modeRank(m models.ThermalMode) int {
	switch m {
	case models.ModeSilent:
		return 0
	case models.ModeStandard:
		return 1
	case models.ModeBoost:
		return 2
	default:
		return 1
	}
}

// evaluateCurveProfile calculates the appropriate thermal mode based on active curve profile points,
// current CPU temperature, current active mode, and default mode fallback with asymmetric hysteresis deadbands.
func evaluateCurveProfile(profile models.CurveProfile, temp float64, currentMode models.ThermalMode, defaultMode models.ThermalMode) models.ThermalMode {
	var hasSilent, hasStandard, hasBoost bool
	var silentMax float64 = 0
	var standardMin float64 = 1000
	var standardMax float64 = 0
	var boostMin float64 = 1000

	for _, pt := range profile.Points {
		mode := pt.Mode
		if mode == "" {
			if pt.PWM < 100 {
				mode = models.ModeSilent
			} else if pt.PWM < 200 {
				mode = models.ModeStandard
			} else {
				mode = models.ModeBoost
			}
		}

		t := float64(pt.TempC)
		switch mode {
		case models.ModeSilent:
			hasSilent = true
			if t > silentMax {
				silentMax = t
			}
		case models.ModeStandard:
			hasStandard = true
			if t < standardMin {
				standardMin = t
			}
			if t > standardMax {
				standardMax = t
			}
		case models.ModeBoost:
			hasBoost = true
			if t < boostMin {
				boostMin = t
			}
		}
	}

	// Calculate thresholds
	boostUp := AutoCurveHighTemp
	if hasBoost {
		boostUp = boostMin
	}

	// boostDown: temperature below which Boost drops back down to Standard.
	// Follows standardMin (e.g. 55°C for balanced, 70°C for quiet) or boostUp - 5°C if no standard mode.
	boostDown := AutoCurveLowTemp
	if hasStandard {
		boostDown = standardMin
		if boostDown >= boostUp {
			boostDown = boostUp - 5.0
		}
	} else {
		boostDown = boostUp - 5.0
	}

	standardUp := AutoCurveLowTemp
	if hasStandard {
		standardUp = standardMin
	} else if hasSilent {
		standardUp = silentMax + 5.0
	}

	// silentDown: temperature below which standard drops back down to silent
	var silentDown float64 = 0
	if hasSilent {
		silentDown = silentMax
		// Ensure minimum deadband of 3°C between standardUp and silentDown
		if standardUp-silentDown < 3.0 {
			silentDown = standardUp - 3.0
		}
	}

	// 1. Immediate Up-step to Boost if hot (cooling safety priority)
	if temp >= boostUp {
		return models.ModeBoost
	}

	// 2. Immediate Up-step from Silent to Standard if warming up
	if currentMode == models.ModeSilent && temp >= standardUp {
		return models.ModeStandard
	}

	// 3. Down-step from Boost when cooled below boostDown hysteresis threshold
	if currentMode == models.ModeBoost {
		if temp <= boostDown {
			if hasSilent && temp <= silentDown {
				return models.ModeSilent
			}
			return models.ModeStandard
		}
		return models.ModeBoost
	}

	// 4. Down-step from Standard to Silent when cooled below silentDown
	if currentMode == models.ModeStandard {
		if hasSilent && temp <= silentDown {
			return models.ModeSilent
		}
		return models.ModeStandard
	}

	// 5. If currentMode was empty/uninitialized, pick initial state
	if currentMode == "" {
		if temp >= boostUp {
			return models.ModeBoost
		}
		if hasSilent && temp < standardUp {
			return models.ModeSilent
		}
		return models.ModeStandard
	}

	return currentMode
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
