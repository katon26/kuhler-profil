package tui_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/godbus/dbus/v5"

	"koolthing/pkg/dbusapi"
	"koolthing/pkg/models"
	"koolthing/pkg/tui"
)

// mockDBusCaller is a test helper satisfying dbusapi.DBusCaller.
type mockDBusCaller struct {
	callFunc func(method string, args ...interface{}) *dbus.Call
}

func (m *mockDBusCaller) Call(method string, flags dbus.Flags, args ...interface{}) *dbus.Call {
	if m.callFunc != nil {
		return m.callFunc(method, args...)
	}
	return &dbus.Call{Err: nil}
}

func (m *mockDBusCaller) CallWithContext(ctx context.Context, method string, flags dbus.Flags, args ...interface{}) *dbus.Call {
	return m.Call(method, flags, args...)
}

func TestRenderTempGauge(t *testing.T) {
	tests := []struct {
		name     string
		temp     float64
		width    int
		contains string
	}{
		{
			name:     "Normal temperature",
			temp:     65.0,
			width:    30,
			contains: "65.0°C",
		},
		{
			name:     "Cool temperature",
			temp:     42.5,
			width:    25,
			contains: "42.5°C",
		},
		{
			name:     "Hot critical temperature",
			temp:     92.0,
			width:    20,
			contains: "92.0°C",
		},
		{
			name:     "Zero or narrow width",
			temp:     50.0,
			width:    5,
			contains: "50.0°C",
		},
		{
			name:     "Negative or zero temp clamped",
			temp:     0.0,
			width:    20,
			contains: "0.0°C",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gauge := tui.RenderTempGauge(tt.temp, tt.width)
			if len(gauge) == 0 {
				t.Fatalf("RenderTempGauge(%v, %v) returned empty string", tt.temp, tt.width)
			}
			if !strings.Contains(gauge, tt.contains) {
				t.Errorf("RenderTempGauge(%v, %v) = %q; want containing %q", tt.temp, tt.width, gauge, tt.contains)
			}
		})
	}
}

func TestRenderFanGauge(t *testing.T) {
	tests := []struct {
		name     string
		rpm      int32
		maxRPM   int32
		width    int
		contains string
	}{
		{
			name:     "Active RPM",
			rpm:      3600,
			maxRPM:   5500,
			width:    25,
			contains: "3600 RPM",
		},
		{
			name:     "Zero RPM / Idle Fan",
			rpm:      0,
			maxRPM:   5500,
			width:    20,
			contains: "0 RPM",
		},
		{
			name:     "Over-speed clamped",
			rpm:      6000,
			maxRPM:   5500,
			width:    20,
			contains: "6000 RPM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gauge := tui.RenderFanGauge(tt.rpm, tt.maxRPM, tt.width)
			if len(gauge) == 0 {
				t.Fatalf("RenderFanGauge returned empty string")
			}
			if !strings.Contains(gauge, tt.contains) {
				t.Errorf("RenderFanGauge = %q; want containing %q", gauge, tt.contains)
			}
		})
	}
}

func TestRenderBatteryGauge(t *testing.T) {
	tests := []struct {
		name     string
		percent  int32
		limit    int32
		onAC     bool
		width    int
		contains string
	}{
		{
			name:     "Battery on AC at limit",
			percent:  80,
			limit:    80,
			onAC:     true,
			width:    25,
			contains: "80%",
		},
		{
			name:     "Battery discharging",
			percent:  55,
			limit:    100,
			onAC:     false,
			width:    20,
			contains: "55%",
		},
		{
			name:     "Battery at 60 limit",
			percent:  60,
			limit:    60,
			onAC:     true,
			width:    20,
			contains: "60%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gauge := tui.RenderBatteryGauge(tt.percent, tt.limit, tt.onAC, tt.width)
			if len(gauge) == 0 {
				t.Fatalf("RenderBatteryGauge returned empty string")
			}
			if !strings.Contains(gauge, tt.contains) {
				t.Errorf("RenderBatteryGauge = %q; want containing %q", gauge, tt.contains)
			}
		})
	}
}

func TestRenderDashboard(t *testing.T) {
	status := models.Telemetry{
		CPUTemp:        72.0,
		Fan1RPM:        3600,
		Fan2RPM:        3400,
		BatteryPercent: 80,
		BatteryLimit:   80,
		OnAC:           true,
		ActiveMode:     models.ModeBoost,
		AutoMode:       true,
	}

	// Normal terminal
	view := tui.RenderDashboard(status, 80, 24)
	if len(view) == 0 {
		t.Fatalf("RenderDashboard(80, 24) returned empty string")
	}

	if !strings.Contains(view, "KOOLTHING") {
		t.Errorf("RenderDashboard missing title/banner 'KOOLTHING'")
	}
	if !strings.Contains(view, "Boost") && !strings.Contains(view, "BOOST") {
		t.Errorf("RenderDashboard missing ActiveMode 'Boost'")
	}
	if !strings.Contains(view, "72.0°C") {
		t.Errorf("RenderDashboard missing CPU temperature '72.0°C'")
	}
	if !strings.Contains(view, "3600 RPM") {
		t.Errorf("RenderDashboard missing Fan 1 RPM '3600 RPM'")
	}
	if !strings.Contains(view, "80%") {
		t.Errorf("RenderDashboard missing Battery percent '80%%'")
	}

	// Compact / Narrow terminal
	compactView := tui.RenderDashboard(status, 50, 20)
	if len(compactView) == 0 {
		t.Fatalf("RenderDashboard(50, 20) returned empty string")
	}

	// Wide terminal
	wideView := tui.RenderDashboard(status, 120, 40)
	if len(wideView) == 0 {
		t.Fatalf("RenderDashboard(120, 40) returned empty string")
	}
}

func TestModelLifecycleAndKeybindings(t *testing.T) {
	m := tui.NewModel(nil)

	// Test Init
	cmd := m.Init()
	if cmd == nil {
		t.Errorf("Model.Init() returned nil cmd")
	}

	// Test WindowSizeMsg
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	view := model.View()
	if len(view) == 0 {
		t.Errorf("model.View() returned empty view after resize")
	}

	// Test TelemetryMsg update
	status := models.Telemetry{
		CPUTemp:        58.5,
		Fan1RPM:        2400,
		BatteryPercent: 75,
		BatteryLimit:   80,
		OnAC:           true,
		ActiveMode:     models.ModeStandard,
		AutoMode:       false,
	}
	model, _ = model.Update(tui.TelemetryMsg(status))
	view = model.View()
	if !strings.Contains(view, "58.5°C") {
		t.Errorf("model.View() did not reflect updated telemetry: %s", view)
	}

	// Test Mode key '1' (Silent)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})

	// Test Mode key '2' (Standard)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})

	// Test Mode key '3' (Boost)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})

	// Test Battery Limit key 'b'
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

	// Test Auto Mode key 'a'
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	// Test Refresh key 'r'
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	// Test TickMsg
	model, _ = model.Update(tui.TickMsg(time.Now()))

	// Test StatusMsg update
	model, _ = model.Update(tui.StatusMsg("Custom notification message"))
	view = model.View()
	if !strings.Contains(view, "Custom notification message") {
		t.Errorf("model.View() did not render status message")
	}

	// Test ErrMsg update
	model, _ = model.Update(tui.ErrMsg(errors.New("connection failed")))
	view = model.View()
	if !strings.Contains(view, "connection failed") {
		t.Errorf("model.View() did not render error message")
	}

	// Test Quit key 'q'
	var quitCmd tea.Cmd
	model, quitCmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quitCmd == nil {
		t.Errorf("model.Update('q') did not return quitCmd")
	}
	if !strings.Contains(model.View(), "Exiting KoolThing") {
		t.Errorf("model.View() did not show exit greeting on quit")
	}
}

func TestModelWithMockDBusClient(t *testing.T) {
	caller := &mockDBusCaller{
		callFunc: func(method string, args ...interface{}) *dbus.Call {
			switch method {
			case dbusapi.Interface + ".GetStatus":
				return &dbus.Call{
					Body: []interface{}{
						map[string]interface{}{
							"cpu_temp":        float64(52.0),
							"fan1_rpm":        int32(2800),
							"battery_percent": int32(85),
							"battery_limit":   int32(80),
							"on_ac":           true,
							"active_mode":     "silent",
							"auto_mode":       true,
						},
					},
				}
			case dbusapi.Interface + ".SetThermalMode":
				return &dbus.Call{Err: nil}
			case dbusapi.Interface + ".SetBatteryLimit":
				return &dbus.Call{Err: nil}
			case dbusapi.Interface + ".SetAutoMode":
				return &dbus.Call{Err: nil}
			default:
				return &dbus.Call{Err: nil}
			}
		},
	}

	client := dbusapi.NewCustomClient(caller)
	m := tui.NewModel(client)

	// Test Model.Init with client
	cmd := m.Init()
	if cmd == nil {
		t.Fatalf("Model.Init with client returned nil cmd")
	}

	// Test Mode key with client attached
	var model tea.Model = m
	var setCmd tea.Cmd
	model, setCmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if setCmd == nil {
		t.Errorf("model.Update('3') with client did not return cmd")
	}
	msg := setCmd()
	if _, ok := msg.(tui.StatusMsg); !ok {
		t.Errorf("setThermalModeCmd returned %T; want StatusMsg", msg)
	}

	// Test Battery key with client attached
	model, setCmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if setCmd == nil {
		t.Errorf("model.Update('b') with client did not return cmd")
	}
	msg = setCmd()
	if _, ok := msg.(tui.StatusMsg); !ok {
		t.Errorf("setBatteryLimitCmd returned %T; want StatusMsg", msg)
	}

	// Test Auto mode key with client attached
	model, setCmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if setCmd == nil {
		t.Errorf("model.Update('a') with client did not return cmd")
	}
	msg = setCmd()
	if _, ok := msg.(tui.StatusMsg); !ok {
		t.Errorf("setAutoModeCmd returned %T; want StatusMsg", msg)
	}

	// Test Refresh key with client attached
	model, setCmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if setCmd == nil {
		t.Errorf("model.Update('r') with client did not return cmd")
	}
	msg = setCmd()
	if _, ok := msg.(tui.TelemetryMsg); !ok {
		t.Errorf("fetchTelemetryCmd returned %T; want TelemetryMsg", msg)
	}
}

func TestNewTUI(t *testing.T) {
	prog := tui.NewTUI(nil)
	if prog == nil {
		t.Fatalf("NewTUI(nil) returned nil program")
	}
}
