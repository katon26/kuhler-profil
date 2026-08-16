package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"koolthing/pkg/dbusapi"
	"koolthing/pkg/models"
)

// Message types for Bubbletea loop
type (
	// TickMsg triggers periodic telemetry refresh.
	TickMsg time.Time

	// TelemetryMsg delivers updated live metrics.
	TelemetryMsg models.Telemetry

	// StatusMsg displays a temporary toast/notification to the user.
	StatusMsg string

	// ErrMsg reports an error state.
	ErrMsg error

	// signalSubMsg holds the active signal subscription channel and cleanup function.
	signalSubMsg struct {
		ch      <-chan models.Telemetry
		cleanup func()
	}
)

// Model represents the Bubbletea UI state machine for KoolThing.
type Model struct {
	client          *dbusapi.DBusClient
	telemetry       models.Telemetry
	width           int
	height          int
	statusMsg       string
	statusMsgExpiry time.Time
	err             error
	connected       bool
	quitting        bool
	sigChan         <-chan models.Telemetry
	sigCleanup      func()
}

// NewModel creates an initialized Bubbletea Model.
func NewModel(client *dbusapi.DBusClient) Model {
	initialTelem := models.Telemetry{
		CPUTemp:        45.0,
		Fan1RPM:        2200,
		Fan2RPM:        0,
		BatteryPercent: 80,
		BatteryLimit:   80,
		OnAC:           true,
		ActiveMode:     models.ModeStandard,
		AutoMode:       false,
	}

	return Model{
		client:    client,
		telemetry: initialTelem,
		width:     80,
		height:    24,
		connected: client != nil,
	}
}

// NewTUI initializes and returns a Bubbletea Program with alternate screen and mouse cell mode.
func NewTUI(client *dbusapi.DBusClient) *tea.Program {
	m := NewModel(client)
	return tea.NewProgram(m, tea.WithAltScreen())
}

// Init starts periodic telemetry polling and signal subscriptions.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd

	// Periodic ticker
	cmds = append(cmds, tickCmd())

	if m.client != nil {
		cmds = append(cmds, fetchTelemetryCmd(m.client))
		cmds = append(cmds, subscribeSignalsCmd(m.client))
	}

	return tea.Batch(cmds...)
}

// Update handles incoming messages and user key events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case TickMsg:
		if time.Now().After(m.statusMsgExpiry) {
			m.statusMsg = ""
		}
		var cmds []tea.Cmd
		cmds = append(cmds, tickCmd())
		if m.client != nil {
			cmds = append(cmds, fetchTelemetryCmd(m.client))
		}
		return m, tea.Batch(cmds...)

	case TelemetryMsg:
		m.telemetry = models.Telemetry(msg)
		m.connected = true
		m.err = nil
		return m, nil

	case signalSubMsg:
		m.sigChan = msg.ch
		m.sigCleanup = msg.cleanup
		return m, waitForSignalCmd(m.sigChan)

	case StatusMsg:
		m.statusMsg = string(msg)
		m.statusMsgExpiry = time.Now().Add(3 * time.Second)
		return m, nil

	case ErrMsg:
		m.err = msg
		m.statusMsg = fmt.Sprintf("Error: %v", msg)
		m.statusMsgExpiry = time.Now().Add(4 * time.Second)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "Q", "esc":
			m.quitting = true
			if m.sigCleanup != nil {
				m.sigCleanup()
			}
			return m, tea.Quit

		case "1":
			m.telemetry.ActiveMode = models.ModeSilent
			m.statusMsg = "Switched to Silent mode"
			m.statusMsgExpiry = time.Now().Add(2 * time.Second)
			if m.client != nil {
				return m, setThermalModeCmd(m.client, models.ModeSilent)
			}
			return m, nil

		case "2":
			m.telemetry.ActiveMode = models.ModeStandard
			m.statusMsg = "Switched to Standard mode"
			m.statusMsgExpiry = time.Now().Add(2 * time.Second)
			if m.client != nil {
				return m, setThermalModeCmd(m.client, models.ModeStandard)
			}
			return m, nil

		case "3":
			m.telemetry.ActiveMode = models.ModeBoost
			m.statusMsg = "Switched to Boost mode"
			m.statusMsgExpiry = time.Now().Add(2 * time.Second)
			if m.client != nil {
				return m, setThermalModeCmd(m.client, models.ModeBoost)
			}
			return m, nil

		case "b", "B":
			nextLimit := nextBatteryLimit(m.telemetry.BatteryLimit)
			m.telemetry.BatteryLimit = nextLimit
			m.statusMsg = fmt.Sprintf("Battery limit set to %d%%", nextLimit)
			m.statusMsgExpiry = time.Now().Add(2 * time.Second)
			if m.client != nil {
				return m, setBatteryLimitCmd(m.client, nextLimit)
			}
			return m, nil

		case "a", "A":
			m.telemetry.AutoMode = !m.telemetry.AutoMode
			stateStr := "ENABLED"
			if !m.telemetry.AutoMode {
				stateStr = "DISABLED"
			}
			m.statusMsg = fmt.Sprintf("Auto-curve governor %s", stateStr)
			m.statusMsgExpiry = time.Now().Add(2 * time.Second)
			if m.client != nil {
				return m, setAutoModeCmd(m.client, m.telemetry.AutoMode)
			}
			return m, nil

		case "r", "R":
			m.statusMsg = "Refreshing telemetry..."
			m.statusMsgExpiry = time.Now().Add(1 * time.Second)
			if m.client != nil {
				return m, fetchTelemetryCmd(m.client)
			}
			return m, nil
		}
	}

	return m, nil
}

// View renders the TUI screen.
func (m Model) View() string {
	if m.quitting {
		return "Exiting KoolThing TUI. Goodbye!\n"
	}

	dashboard := RenderDashboard(m.telemetry, m.width, m.height)

	if len(m.statusMsg) > 0 {
		var toast string
		if m.err != nil {
			toast = ErrorToastStyle.Render("✖ " + m.statusMsg)
		} else {
			toast = ToastStyle.Render("✔ " + m.statusMsg)
		}
		return lipgloss.JoinVertical(lipgloss.Left, dashboard, "", toast)
	}

	return dashboard
}

// Helper commands

func tickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func fetchTelemetryCmd(client *dbusapi.DBusClient) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		status, err := client.GetStatus()
		if err != nil {
			return ErrMsg(err)
		}
		return TelemetryMsg(status)
	}
}

func setThermalModeCmd(client *dbusapi.DBusClient, mode models.ThermalMode) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		err := client.SetThermalMode(string(mode))
		if err != nil {
			return ErrMsg(err)
		}
		return StatusMsg(fmt.Sprintf("Profile updated: %s", mode))
	}
}

func setBatteryLimitCmd(client *dbusapi.DBusClient, limit int32) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		err := client.SetBatteryLimit(limit)
		if err != nil {
			return ErrMsg(err)
		}
		return StatusMsg(fmt.Sprintf("Battery threshold set: %d%%", limit))
	}
}

func setAutoModeCmd(client *dbusapi.DBusClient, enabled bool) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		err := client.SetAutoMode(enabled)
		if err != nil {
			return ErrMsg(err)
		}
		if enabled {
			return StatusMsg("Auto-curve governor activated")
		}
		return StatusMsg("Auto-curve governor deactivated")
	}
}

func subscribeSignalsCmd(client *dbusapi.DBusClient) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		ch, cleanup, err := client.SubscribeSignals(context.Background())
		if err != nil {
			return nil // fallback to polling
		}
		return signalSubMsg{ch: ch, cleanup: cleanup}
	}
}

func waitForSignalCmd(ch <-chan models.Telemetry) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return nil
		}
		telem, ok := <-ch
		if !ok {
			return nil
		}
		return TelemetryMsg(telem)
	}
}

func nextBatteryLimit(current int32) int32 {
	switch current {
	case 60:
		return 80
	case 80:
		return 100
	case 100:
		return 60
	default:
		return 80
	}
}
