package tui_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/godbus/dbus/v5"

	"kuhlerprofil/pkg/dbusapi"
	"kuhlerprofil/pkg/models"
	"kuhlerprofil/pkg/tui"
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

	if !strings.Contains(view, "KühlerProfil ASUS Control") {
		t.Errorf("RenderDashboard missing header title 'KühlerProfil ASUS Control'")
	}
	if !strings.Contains(view, "KÜHLERPROFIL") {
		t.Errorf("RenderDashboard missing title/banner 'KÜHLERPROFIL'")
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
	if !strings.Contains(view, "COOLDOWN") {
		t.Errorf("RenderDashboard missing Cooldown indicator 'COOLDOWN'")
	}

	// Compact / Narrow terminal
	compactView := tui.RenderDashboard(status, 50, 20)
	if len(compactView) == 0 {
		t.Fatalf("RenderDashboard(50, 20) returned empty string")
	}
	if !strings.Contains(compactView, "KühlerProfil ASUS Control") {
		t.Errorf("RenderDashboard compact missing header title 'KühlerProfil ASUS Control'")
	}
	if !strings.Contains(compactView, "KÜHLERPROFIL") {
		t.Errorf("RenderDashboard compact missing title/banner 'KÜHLERPROFIL'")
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

	// Test Cooldown Mode key 'c'
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

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
	if !strings.Contains(model.View(), "Exiting KühlerProfil") {
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
			case dbusapi.Interface + ".SetCooldownMode":
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

	// Test Cooldown mode key with client attached
	model, setCmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if setCmd == nil {
		t.Errorf("model.Update('c') with client did not return cmd")
	}
	msg = setCmd()
	if msg != nil {
		t.Errorf("setCooldownModeCmd returned %v; want nil on success", msg)
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

func TestLogoBanner(t *testing.T) {
	t.Run("Compact banner", func(t *testing.T) {
		banner := tui.LogoBanner(true)
		if !strings.Contains(banner, "⚡ KÜHLERPROFIL") {
			t.Errorf("compact banner missing title: %q", banner)
		}
	})

	t.Run("Full banner line count and dimensions", func(t *testing.T) {
		banner := tui.LogoBanner(false)
		lines := strings.Split(strings.TrimRight(banner, "\n"), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines in full banner, got %d", len(lines))
		}

		if !strings.Contains(banner, "KÜHLERPROFIL") {
			t.Errorf("full banner missing subtitle KÜHLERPROFIL")
		}
		if !strings.Contains(banner, "ASUS Control") {
			t.Errorf("full banner missing subtitle ASUS Control")
		}

		// Ensure 'F' glyph row 2 is present ("█▀ ")
		if !strings.Contains(lines[1], "█▀ ") {
			t.Errorf("expected letter 'F' glyph row 2 '█▀ ' in line 2: %q", lines[1])
		}
	})
}

func TestThemePalettes(t *testing.T) {
	if len(tui.AvailableThemes) < 3 {
		t.Fatalf("expected at least 3 themes, got %d", len(tui.AvailableThemes))
	}
	for i, theme := range tui.AvailableThemes {
		banner := tui.LogoBannerWithTheme(false, i)
		if !strings.Contains(banner, "KÜHLERPROFIL") {
			t.Errorf("theme %s banner missing KÜHLERPROFIL", theme.Name)
		}
	}
}

func TestModelMouseInteraction(t *testing.T) {
	var model tea.Model = tui.NewModel(nil)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})


	// 1. Mouse click on Logo cycles theme
	clickLogo := tea.MouseMsg{
		X:      10,
		Y:      1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	model, _ = model.Update(clickLogo)
	view := model.View()
	if !strings.Contains(view, "Theme: Cyberpunk Violet") {
		t.Errorf("clicking logo did not switch to theme Cyberpunk Violet: %s", view)
	}

	// 2. Mouse click on Silent mode pill
	clickSilent := tea.MouseMsg{
		X:      44,
		Y:      13,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	model, _ = model.Update(clickSilent)
	view = model.View()
	if !strings.Contains(view, "Switched to Silent mode") {
		t.Errorf("clicking Silent pill did not switch mode: %s", view)
	}

	// 3. Mouse click on Standard mode pill
	clickStd := tea.MouseMsg{
		X:      55,
		Y:      13,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	model, _ = model.Update(clickStd)
	view = model.View()
	if !strings.Contains(view, "Switched to Standard mode") {
		t.Errorf("clicking Standard pill did not switch mode: %s", view)
	}

	// 4. Mouse click on Boost mode pill
	clickBoost := tea.MouseMsg{
		X:      70,
		Y:      13,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	model, _ = model.Update(clickBoost)
	view = model.View()
	if !strings.Contains(view, "Switched to Boost mode") {
		t.Errorf("clicking Boost pill did not switch mode: %s", view)
	}

	// 5. Mouse click on Battery card cycles limit
	clickBattery := tea.MouseMsg{
		X:      15,
		Y:      12,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	model, _ = model.Update(clickBattery)
	view = model.View()
	if !strings.Contains(view, "Battery limit set to 100%") {
		t.Errorf("clicking battery card did not cycle to 100%%: %s", view)
	}

	// 6. Mouse click on Governor badge toggles governor
	clickGov := tea.MouseMsg{
		X:      46,
		Y:      15,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	model, _ = model.Update(clickGov)
	view = model.View()
	if !strings.Contains(view, "Auto-curve governor ENABLED") {
		t.Errorf("clicking governor badge did not enable governor: %s", view)
	}

	// 7. Mouse click on Cooldown badge activates cooldown
	clickCD := tea.MouseMsg{
		X:      68,
		Y:      15,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	model, _ = model.Update(clickCD)
	view = model.View()
	if !strings.Contains(view, "Cooldown mode:") {
		t.Errorf("clicking cooldown badge did not trigger cooldown: %s", view)
	}

	// 8. Mouse click on Thermal card refreshes telemetry
	clickRefresh := tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	model, _ = model.Update(clickRefresh)
	view = model.View()
	if !strings.Contains(view, "Refreshing telemetry...") {
		t.Errorf("clicking thermal card did not refresh telemetry: %s", view)
	}

	// 9. Right click or Mouse Motion is ignored
	rightClick := tea.MouseMsg{
		X:      10,
		Y:      1,
		Button: tea.MouseButtonRight,
		Action: tea.MouseActionPress,
	}
	model, _ = model.Update(rightClick)

	mouseMotion := tea.MouseMsg{
		X:      10,
		Y:      1,
		Action: tea.MouseActionMotion,
	}
	model, _ = model.Update(mouseMotion)

	// 10. Verify Keyboard shortcuts still work identically
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if !strings.Contains(model.View(), "Switched to Silent mode") {
		t.Errorf("key '1' failed after mouse events")
	}
}



