package dbusapi

import (
	"context"
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	"kuhlerprofil/pkg/engine"
	"kuhlerprofil/pkg/models"
)

const (
	// Interface is the standard KühlerProfil D-Bus interface name.
	Interface = "org.freedesktop.kuhlerprofil"
	// InterfaceName is an alias for Interface.
	InterfaceName = Interface
	// Path is the standard KühlerProfil D-Bus object path.
	Path = dbus.ObjectPath("/org/freedesktop/kuhlerprofil")
	// ObjectPath is the string representation of Path.
	ObjectPath = "/org/freedesktop/kuhlerprofil"
	// ServiceName is the well-known bus name for KühlerProfil.
	ServiceName = "org.freedesktop.kuhlerprofil"
	// BusName is an alias for ServiceName.
	BusName = ServiceName

	// SignalThermalModeChanged is emitted whenever active thermal mode changes.
	SignalThermalModeChanged = "ThermalModeChanged"
	// SignalBatteryLimitChanged is emitted whenever battery charging threshold changes.
	SignalBatteryLimitChanged = "BatteryLimitChanged"
	// SignalTelemetryTick is emitted on every telemetry refresh interval.
	SignalTelemetryTick = "TelemetryTick"

	// SignalCurveProfileChanged is emitted whenever active curve profile changes.
	SignalCurveProfileChanged = "CurveProfileChanged"

	// SignalThermalModeChangedFull is the fully-qualified member for ThermalModeChanged.
	SignalThermalModeChangedFull = Interface + "." + SignalThermalModeChanged
	// SignalBatteryLimitChangedFull is the fully-qualified member for BatteryLimitChanged.
	SignalBatteryLimitChangedFull = Interface + "." + SignalBatteryLimitChanged
	// SignalTelemetryTickFull is the fully-qualified member for TelemetryTick.
	SignalTelemetryTickFull = Interface + "." + SignalTelemetryTick
	// SignalCurveProfileChangedFull is the fully-qualified member for CurveProfileChanged.
	SignalCurveProfileChangedFull = Interface + "." + SignalCurveProfileChanged
)

// IntrospectionXML provides formal D-Bus introspection definitions for tooling and language bindings.
const IntrospectionXML = `<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN"
"http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">
<node>
  <interface name="org.freedesktop.kuhlerprofil">
    <method name="GetStatus">
      <arg name="status" type="a{sv}" direction="out"/>
    </method>
    <method name="SetThermalMode">
      <arg name="mode" type="s" direction="in"/>
    </method>
    <method name="SetBatteryLimit">
      <arg name="limit" type="i" direction="in"/>
    </method>
    <method name="SetAutoMode">
      <arg name="enabled" type="b" direction="in"/>
    </method>
    <method name="GetCurveProfiles">
      <arg name="profiles" type="a{sa{sv}}" direction="out"/>
    </method>
    <method name="GetActiveCurveProfile">
      <arg name="name" type="s" direction="out"/>
    </method>
    <method name="SetCurveProfile">
      <arg name="name" type="s" direction="in"/>
    </method>
    <method name="GetHardwareFanCurves">
      <arg name="status" type="a{sv}" direction="out"/>
    </method>
    <signal name="ThermalModeChanged">
      <arg name="mode" type="s"/>
    </signal>
    <signal name="BatteryLimitChanged">
      <arg name="limit" type="i"/>
    </signal>
    <signal name="CurveProfileChanged">
      <arg name="name" type="s"/>
    </signal>
    <signal name="TelemetryTick">
      <arg name="status" type="a{sv}"/>
    </signal>
  </interface>
  <interface name="org.freedesktop.DBus.Introspectable">
    <method name="Introspect">
      <arg name="out" type="s" direction="out"/>
    </method>
  </interface>
</node>`

// FormatStatusMap converts a Telemetry struct into a generic map for D-Bus dictionary transmission.
func FormatStatusMap(status models.Telemetry) map[string]interface{} {
	activeCurve := status.ActiveCurveProfile
	if activeCurve == "" {
		activeCurve = "balanced"
	}
	return map[string]interface{}{
		"cpu_temp":              status.CPUTemp,
		"fan1_rpm":              status.Fan1RPM,
		"fan2_rpm":              status.Fan2RPM,
		"battery_percent":       status.BatteryPercent,
		"battery_limit":         status.BatteryLimit,
		"on_ac":                 status.OnAC,
		"active_mode":           string(status.ActiveMode),
		"auto_mode":             status.AutoMode,
		"active_curve_profile":  activeCurve,
		"active_curve":          activeCurve,
		"has_hardware_curve":    status.HasHardwareCurve,
	}
}

// FormatStatusVariantMap converts a Telemetry struct into a D-Bus variant map.
func FormatStatusVariantMap(status models.Telemetry) map[string]dbus.Variant {
	m := FormatStatusMap(status)
	res := make(map[string]dbus.Variant, len(m))
	for k, v := range m {
		res[k] = dbus.MakeVariant(v)
	}
	return res
}

// ParseStatusMap reconstructs a Telemetry struct from a map with type coercion support.
func ParseStatusMap(m map[string]interface{}) (models.Telemetry, error) {
	if len(m) == 0 {
		return models.Telemetry{ActiveMode: models.ModeStandard}, nil
	}

	var telem models.Telemetry
	telem.ActiveMode = models.ModeStandard

	for k, rawVal := range m {
		// Unwrap dbus.Variant if present
		val := rawVal
		if v, ok := rawVal.(dbus.Variant); ok {
			val = v.Value()
		}

		switch k {
		case "cpu_temp":
			switch num := val.(type) {
			case float64:
				telem.CPUTemp = num
			case float32:
				telem.CPUTemp = float64(num)
			case int:
				telem.CPUTemp = float64(num)
			case int64:
				telem.CPUTemp = float64(num)
			}
		case "fan1_rpm":
			telem.Fan1RPM = toInt32(val)
		case "fan2_rpm":
			telem.Fan2RPM = toInt32(val)
		case "battery_percent":
			telem.BatteryPercent = toInt32(val)
		case "battery_limit":
			telem.BatteryLimit = toInt32(val)
		case "on_ac":
			if b, ok := val.(bool); ok {
				telem.OnAC = b
			}
		case "active_mode":
			if s, ok := val.(string); ok && s != "" {
				mode, err := models.ParseThermalMode(s)
				if err != nil {
					return telem, fmt.Errorf("invalid active_mode in status map: %w", err)
				}
				telem.ActiveMode = mode
			}
		case "auto_mode":
			if b, ok := val.(bool); ok {
				telem.AutoMode = b
			}
		case "active_curve_profile":
			if s, ok := val.(string); ok {
				telem.ActiveCurveProfile = s
			}
		case "has_hardware_curve":
			if b, ok := val.(bool); ok {
				telem.HasHardwareCurve = b
			}
		}
	}

	return telem, nil
}

// ParseStatusVariantMap converts a D-Bus variant dictionary to a Telemetry struct.
func ParseStatusVariantMap(m map[string]dbus.Variant) (models.Telemetry, error) {
	raw := make(map[string]interface{}, len(m))
	for k, v := range m {
		raw[k] = v.Value()
	}
	return ParseStatusMap(raw)
}

func toInt32(val interface{}) int32 {
	switch v := val.(type) {
	case int32:
		return v
	case int:
		return int32(v)
	case int64:
		return int32(v)
	case uint32:
		return int32(v)
	case uint:
		return int32(v)
	case float64:
		return int32(v)
	default:
		return 0
	}
}

// DBusServer exposes the KühlerProfil engine to the Linux system bus via D-Bus IPC.
type DBusServer struct {
	mu         sync.Mutex
	eng        *engine.Engine
	conn       *dbus.Conn
	cancel     context.CancelFunc
	subChan    <-chan models.Telemetry
	wg         sync.WaitGroup
	lastMode   models.ThermalMode
	lastLimit  int32
	isExported bool
}

// NewServer instantiates a DBusServer wrapping the provided Engine.
func NewServer(eng *engine.Engine) *DBusServer {
	return &DBusServer{
		eng: eng,
	}
}

// Export registers the D-Bus object methods, properties, and introspection on the connection.
func (s *DBusServer) Export(conn *dbus.Conn, eng *engine.Engine) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.conn = conn
	if eng != nil {
		s.eng = eng
	}

	// Export KühlerProfil interface methods
	if err := conn.Export(s, Path, Interface); err != nil {
		return fmt.Errorf("failed to export D-Bus interface %s: %w", Interface, err)
	}

	// Export Introspectable interface
	if err := conn.Export(introspect.Introspectable(IntrospectionXML), Path, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("failed to export D-Bus introspection: %w", err)
	}

	s.isExported = true

	// Request well-known bus name
	reply, err := conn.RequestName(ServiceName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("failed to request D-Bus name %s: %w", ServiceName, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("bus name %s is already taken by another service", ServiceName)
	}

	return nil
}

// StartSignalBroadcaster starts a background loop listening to engine telemetry updates
// and emitting D-Bus signals for changes and periodic ticks.
func (s *DBusServer) StartSignalBroadcaster(ctx context.Context, conn *dbus.Conn) {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return
	}

	if conn != nil {
		s.conn = conn
	}
	broadcasterCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	var subChan <-chan models.Telemetry
	if s.eng != nil {
		subChan = s.eng.SubscribeTelemetry()
		s.subChan = subChan
		initial := s.eng.GetTelemetry()
		s.lastMode = initial.ActiveMode
		s.lastLimit = initial.BatteryLimit
	}
	s.mu.Unlock()

	if subChan == nil {
		return
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-broadcasterCtx.Done():
				return
			case telem, ok := <-subChan:
				if !ok {
					return
				}
				s.emitTelemetrySignals(telem)
			}
		}
	}()
}

// emitTelemetrySignals dispatches D-Bus signals for telemetry tick and state transitions.
func (s *DBusServer) emitTelemetrySignals(telem models.Telemetry) {
	s.mu.Lock()
	conn := s.conn
	lastMode := s.lastMode
	lastLimit := s.lastLimit

	modeChanged := telem.ActiveMode != lastMode
	if modeChanged {
		s.lastMode = telem.ActiveMode
	}

	limitChanged := telem.BatteryLimit != lastLimit
	if limitChanged {
		s.lastLimit = telem.BatteryLimit
	}
	s.mu.Unlock()

	if conn == nil {
		return
	}

	// 1. Emit TelemetryTick signal
	statusVariant := FormatStatusVariantMap(telem)
	_ = conn.Emit(Path, SignalTelemetryTickFull, statusVariant)

	// 2. Emit ThermalModeChanged if profile transitioned
	if modeChanged {
		_ = conn.Emit(Path, SignalThermalModeChangedFull, string(telem.ActiveMode))
	}

	// 3. Emit BatteryLimitChanged if threshold changed
	if limitChanged {
		_ = conn.Emit(Path, SignalBatteryLimitChangedFull, telem.BatteryLimit)
	}
}

// Stop terminates background signal broadcasting and unsubscribes from the engine.
func (s *DBusServer) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	subChan := s.subChan
	s.subChan = nil
	eng := s.eng
	s.mu.Unlock()

	s.wg.Wait()

	if eng != nil && subChan != nil {
		eng.UnsubscribeTelemetry(subChan)
	}
}

// --- D-Bus Exported RPC Method Handlers ---

// GetStatus returns the current telemetry snapshot as a dictionary map.
func (s *DBusServer) GetStatus() (map[string]interface{}, *dbus.Error) {
	s.mu.Lock()
	eng := s.eng
	s.mu.Unlock()

	if eng == nil {
		return nil, dbus.NewError("org.freedesktop.kuhlerprofil.Error.Unavailable", []interface{}{"engine not ready"})
	}

	telem := eng.GetTelemetry()
	return FormatStatusMap(telem), nil
}

// SetThermalMode switches the active ASUS thermal profile.
func (s *DBusServer) SetThermalMode(mode string) *dbus.Error {
	m, err := models.ParseThermalMode(mode)
	if err != nil {
		return dbus.NewError("org.freedesktop.kuhlerprofil.Error.InvalidMode", []interface{}{err.Error()})
	}

	s.mu.Lock()
	eng := s.eng
	s.mu.Unlock()

	if eng == nil {
		return dbus.NewError("org.freedesktop.kuhlerprofil.Error.Unavailable", []interface{}{"engine not ready"})
	}

	if err := eng.SetMode(m); err != nil {
		return dbus.NewError("org.freedesktop.kuhlerprofil.Error.HardwareFailure", []interface{}{err.Error()})
	}

	return nil
}

// SetBatteryLimit configures the ASUS battery charge threshold (60%, 80%, 100%).
func (s *DBusServer) SetBatteryLimit(limit int32) *dbus.Error {
	if err := models.ValidateBatteryLimit(limit); err != nil {
		return dbus.NewError("org.freedesktop.kuhlerprofil.Error.InvalidLimit", []interface{}{err.Error()})
	}

	s.mu.Lock()
	eng := s.eng
	s.mu.Unlock()

	if eng == nil {
		return dbus.NewError("org.freedesktop.kuhlerprofil.Error.Unavailable", []interface{}{"engine not ready"})
	}

	if err := eng.SetBatteryLimit(limit); err != nil {
		return dbus.NewError("org.freedesktop.kuhlerprofil.Error.HardwareFailure", []interface{}{err.Error()})
	}

	return nil
}

// SetAutoMode toggles the automatic thermal curve governor.
func (s *DBusServer) SetAutoMode(enabled bool) *dbus.Error {
	s.mu.Lock()
	eng := s.eng
	s.mu.Unlock()

	if eng == nil {
		return dbus.NewError("org.freedesktop.kuhlerprofil.Error.Unavailable", []interface{}{"engine not ready"})
	}

	if err := eng.SetAutoMode(enabled); err != nil {
		return dbus.NewError("org.freedesktop.kuhlerprofil.Error.HardwareFailure", []interface{}{err.Error()})
	}

	return nil
}

// GetCurveProfiles returns all available curve profiles as a dictionary.
func (s *DBusServer) GetCurveProfiles() (map[string]map[string]dbus.Variant, *dbus.Error) {
	s.mu.Lock()
	eng := s.eng
	s.mu.Unlock()

	if eng == nil {
		return nil, dbus.NewError("org.freedesktop.kuhlerprofil.Error.Unavailable", []interface{}{"engine not ready"})
	}

	profiles := eng.GetCurveProfiles()
	res := make(map[string]map[string]dbus.Variant, len(profiles))
	for name, prof := range profiles {
		pMap := map[string]dbus.Variant{
			"name":        dbus.MakeVariant(prof.Name),
			"description": dbus.MakeVariant(prof.Description),
		}
		res[name] = pMap
	}
	return res, nil
}

// GetActiveCurveProfile returns the active curve profile name.
func (s *DBusServer) GetActiveCurveProfile() (string, *dbus.Error) {
	s.mu.Lock()
	eng := s.eng
	s.mu.Unlock()

	if eng == nil {
		return "", dbus.NewError("org.freedesktop.kuhlerprofil.Error.Unavailable", []interface{}{"engine not ready"})
	}

	return eng.GetActiveCurveProfile(), nil
}

// SetCurveProfile activates the requested curve profile by name.
func (s *DBusServer) SetCurveProfile(name string) *dbus.Error {
	s.mu.Lock()
	eng := s.eng
	conn := s.conn
	s.mu.Unlock()

	if eng == nil {
		return dbus.NewError("org.freedesktop.kuhlerprofil.Error.Unavailable", []interface{}{"engine not ready"})
	}

	if err := eng.SetCurveProfile(name); err != nil {
		return dbus.NewError("org.freedesktop.kuhlerprofil.Error.InvalidProfile", []interface{}{err.Error()})
	}

	if conn != nil {
		_ = conn.Emit(Path, SignalCurveProfileChangedFull, name)
	}

	return nil
}

// GetHardwareFanCurves returns whether hardware ACPI fan curves are supported on this machine.
func (s *DBusServer) GetHardwareFanCurves() (map[string]dbus.Variant, *dbus.Error) {
	s.mu.Lock()
	eng := s.eng
	s.mu.Unlock()

	if eng == nil {
		return nil, dbus.NewError("org.freedesktop.kuhlerprofil.Error.Unavailable", []interface{}{"engine not ready"})
	}

	caps := eng.GetDriver().GetHardwareCurveCaps()
	res := map[string]dbus.Variant{
		"supported": dbus.MakeVariant(caps.Supported),
		"fan_count": dbus.MakeVariant(int32(caps.FanCount)),
	}
	return res, nil
}

