package dbusapi

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"

	"kuhlerprofil/pkg/models"
)

// DBusCaller abstracts the low-level D-Bus bus object Call interface for testing and decoupling.
type DBusCaller interface {
	Call(method string, flags dbus.Flags, args ...interface{}) *dbus.Call
	CallWithContext(ctx context.Context, method string, flags dbus.Flags, args ...interface{}) *dbus.Call
}

// DBusClient provides a typed client wrapper to interact with the KühlerProfil daemon over D-Bus IPC.
type DBusClient struct {
	conn     *dbus.Conn
	caller   DBusCaller
	ownsConn bool
}

// NewClient connects to the Linux System Bus (with Session Bus fallback for user test environments)
// and returns a ready-to-use DBusClient.
func NewClient() (*DBusClient, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		// Fallback to session bus (useful in rootless developer test sessions)
		sessionConn, sessionErr := dbus.ConnectSessionBus()
		if sessionErr != nil {
			return nil, fmt.Errorf("failed to connect to system bus (%v) or session bus (%v)", err, sessionErr)
		}
		conn = sessionConn
	}

	obj := conn.Object(ServiceName, Path)
	return &DBusClient{
		conn:     conn,
		caller:   obj,
		ownsConn: true,
	}, nil
}

// NewClientWithConn creates a DBusClient using an existing D-Bus connection.
func NewClientWithConn(conn *dbus.Conn) *DBusClient {
	if conn == nil {
		return &DBusClient{}
	}
	obj := conn.Object(ServiceName, Path)
	return &DBusClient{
		conn:     conn,
		caller:   obj,
		ownsConn: false,
	}
}

// NewCustomClient instantiates a DBusClient backed by a custom DBusCaller implementation.
func NewCustomClient(caller DBusCaller) *DBusClient {
	return &DBusClient{
		caller: caller,
	}
}

// GetStatus queries the KühlerProfil daemon for live telemetry and hardware status.
func (c *DBusClient) GetStatus() (models.Telemetry, error) {
	if c.caller == nil {
		return models.Telemetry{}, fmt.Errorf("dbus client is not connected")
	}

	call := c.caller.Call(Interface+".GetStatus", 0)
	if call.Err != nil {
		return models.Telemetry{}, fmt.Errorf("failed to get status over D-Bus: %w", call.Err)
	}

	if len(call.Body) == 0 {
		return models.Telemetry{}, fmt.Errorf("empty response received from D-Bus GetStatus")
	}

	switch val := call.Body[0].(type) {
	case map[string]interface{}:
		return ParseStatusMap(val)
	case map[string]dbus.Variant:
		return ParseStatusVariantMap(val)
	default:
		return models.Telemetry{}, fmt.Errorf("unexpected response type %T from D-Bus GetStatus", val)
	}
}

// SetThermalMode instructs the daemon to switch the active ASUS thermal profile.
func (c *DBusClient) SetThermalMode(mode string) error {
	if c.caller == nil {
		return fmt.Errorf("dbus client is not connected")
	}

	call := c.caller.Call(Interface+".SetThermalMode", 0, mode)
	if call.Err != nil {
		return fmt.Errorf("failed to set thermal mode over D-Bus: %w", call.Err)
	}
	return nil
}

// SetBatteryLimit instructs the daemon to update the battery charge end threshold.
func (c *DBusClient) SetBatteryLimit(limit int32) error {
	if c.caller == nil {
		return fmt.Errorf("dbus client is not connected")
	}

	call := c.caller.Call(Interface+".SetBatteryLimit", 0, limit)
	if call.Err != nil {
		return fmt.Errorf("failed to set battery limit over D-Bus: %w", call.Err)
	}
	return nil
}

// SetAutoMode toggles the daemon's automated thermal curve governor.
func (c *DBusClient) SetAutoMode(enabled bool) error {
	if c.caller == nil {
		return fmt.Errorf("dbus client is not connected")
	}

	call := c.caller.Call(Interface+".SetAutoMode", 0, enabled)
	if call.Err != nil {
		return fmt.Errorf("failed to set auto mode over D-Bus: %w", call.Err)
	}
	return nil
}

// GetCurveProfiles queries available curve profiles over D-Bus.
func (c *DBusClient) GetCurveProfiles() (map[string]models.CurveProfile, error) {
	if c.caller == nil {
		return nil, fmt.Errorf("dbus client is not connected")
	}

	call := c.caller.Call(Interface+".GetCurveProfiles", 0)
	if call.Err != nil {
		return nil, fmt.Errorf("failed to get curve profiles over D-Bus: %w", call.Err)
	}

	if len(call.Body) == 0 {
		return nil, fmt.Errorf("empty response from GetCurveProfiles")
	}

	res := make(map[string]models.CurveProfile)
	if rawMap, ok := call.Body[0].(map[string]map[string]dbus.Variant); ok {
		for name, pMap := range rawMap {
			desc := ""
			if v, ok := pMap["description"]; ok {
				if s, ok := v.Value().(string); ok {
					desc = s
				}
			}
			res[name] = models.CurveProfile{
				Name:        name,
				Description: desc,
			}
		}
		return res, nil
	}
	return res, nil
}

// GetActiveCurveProfile queries the active curve profile name over D-Bus.
func (c *DBusClient) GetActiveCurveProfile() (string, error) {
	if c.caller == nil {
		return "", fmt.Errorf("dbus client is not connected")
	}

	call := c.caller.Call(Interface+".GetActiveCurveProfile", 0)
	if call.Err != nil {
		return "", fmt.Errorf("failed to get active curve profile over D-Bus: %w", call.Err)
	}

	if len(call.Body) == 0 {
		return "", fmt.Errorf("empty response from GetActiveCurveProfile")
	}

	if name, ok := call.Body[0].(string); ok {
		return name, nil
	}
	return "", fmt.Errorf("unexpected response type %T from GetActiveCurveProfile", call.Body[0])
}

// SetCurveProfile changes the active curve profile over D-Bus.
func (c *DBusClient) SetCurveProfile(name string) error {
	if c.caller == nil {
		return fmt.Errorf("dbus client is not connected")
	}

	call := c.caller.Call(Interface+".SetCurveProfile", 0, name)
	if call.Err != nil {
		return fmt.Errorf("failed to set curve profile over D-Bus: %w", call.Err)
	}
	return nil
}

// GetHardwareFanCurves queries hardware fan curve capabilities and status over D-Bus.
func (c *DBusClient) GetHardwareFanCurves() (map[string]dbus.Variant, error) {
	if c.caller == nil {
		return nil, fmt.Errorf("dbus client is not connected")
	}

	call := c.caller.Call(Interface+".GetHardwareFanCurves", 0)
	if call.Err != nil {
		return nil, fmt.Errorf("failed to get hardware fan curves over D-Bus: %w", call.Err)
	}

	if len(call.Body) == 0 {
		return nil, fmt.Errorf("empty response from GetHardwareFanCurves")
	}

	if res, ok := call.Body[0].(map[string]dbus.Variant); ok {
		return res, nil
	}
	return nil, fmt.Errorf("unexpected response type %T from GetHardwareFanCurves", call.Body[0])
}

// SubscribeSignals registers D-Bus match rules and streams telemetry snapshots and mode changes.
func (c *DBusClient) SubscribeSignals(ctx context.Context) (<-chan models.Telemetry, func(), error) {
	if c.conn == nil {
		return nil, nil, fmt.Errorf("signal subscription requires an active D-Bus connection")
	}

	rule := fmt.Sprintf("type='signal',interface='%s',path='%s'", Interface, ObjectPath)
	call := c.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule)
	if call.Err != nil {
		return nil, nil, fmt.Errorf("failed to add D-Bus match rule for signals: %w", call.Err)
	}

	sigChan := make(chan *dbus.Signal, 16)
	c.conn.Signal(sigChan)

	outChan := make(chan models.Telemetry, 16)

	subCtx, cancel := context.WithCancel(ctx)

	go func() {
		defer close(outChan)
		for {
			select {
			case <-subCtx.Done():
				return
			case sig, ok := <-sigChan:
				if !ok {
					return
				}
				if sig == nil {
					continue
				}

				if sig.Name == SignalTelemetryTickFull && len(sig.Body) > 0 {
					if m, ok := sig.Body[0].(map[string]dbus.Variant); ok {
						if telem, err := ParseStatusVariantMap(m); err == nil {
							select {
							case outChan <- telem:
							default:
							}
						}
					} else if m, ok := sig.Body[0].(map[string]interface{}); ok {
						if telem, err := ParseStatusMap(m); err == nil {
							select {
							case outChan <- telem:
							default:
							}
						}
					}
				}
			}
		}
	}()

	cleanup := func() {
		cancel()
		c.conn.RemoveSignal(sigChan)
		_ = c.conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, rule)
	}

	return outChan, cleanup, nil
}

// Close closes the underlying D-Bus connection if owned by this client.
func (c *DBusClient) Close() error {
	if c.ownsConn && c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
