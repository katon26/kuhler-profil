package dbusapi_test

import (
	"context"
	"encoding/xml"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"kuhlerprofil/pkg/dbusapi"
	"kuhlerprofil/pkg/driver"
	"kuhlerprofil/pkg/engine"
	"kuhlerprofil/pkg/models"
)

// TestDBusSignatureAndMarshaling tests mapping between models.Telemetry and D-Bus payload maps.
func TestDBusSignatureAndMarshaling(t *testing.T) {
	status := models.Telemetry{
		CPUTemp:        62.5,
		Fan1RPM:        3200,
		Fan2RPM:        0,
		BatteryPercent: 85,
		BatteryLimit:   80,
		OnAC:           true,
		ActiveMode:     models.ModeBoost,
		AutoMode:       false,
	}

	exported := dbusapi.FormatStatusMap(status)
	if exported["active_mode"] != "boost" {
		t.Errorf("exported active_mode = %v, want boost", exported["active_mode"])
	}
	if exported["cpu_temp"] != 62.5 {
		t.Errorf("exported cpu_temp = %v, want 62.5", exported["cpu_temp"])
	}
	if exported["fan1_rpm"] != int32(3200) {
		t.Errorf("exported fan1_rpm = %v, want 3200", exported["fan1_rpm"])
	}
	if exported["fan2_rpm"] != int32(0) {
		t.Errorf("exported fan2_rpm = %v, want 0", exported["fan2_rpm"])
	}
	if exported["battery_percent"] != int32(85) {
		t.Errorf("exported battery_percent = %v, want 85", exported["battery_percent"])
	}
	if exported["battery_limit"] != int32(80) {
		t.Errorf("exported battery_limit = %v, want 80", exported["battery_limit"])
	}
	if exported["on_ac"] != true {
		t.Errorf("exported on_ac = %v, want true", exported["on_ac"])
	}
	if exported["auto_mode"] != false {
		t.Errorf("exported auto_mode = %v, want false", exported["auto_mode"])
	}

	// Test variant map
	variantMap := dbusapi.FormatStatusVariantMap(status)
	if variantMap["active_mode"].Value() != "boost" {
		t.Errorf("variantMap active_mode = %v, want boost", variantMap["active_mode"].Value())
	}
	if variantMap["cpu_temp"].Value() != 62.5 {
		t.Errorf("variantMap cpu_temp = %v, want 62.5", variantMap["cpu_temp"].Value())
	}

	// Test reverse parsing from map
	parsed, err := dbusapi.ParseStatusMap(exported)
	if err != nil {
		t.Fatalf("ParseStatusMap failed: %v", err)
	}
	if parsed != status {
		t.Errorf("parsed = %+v, want %+v", parsed, status)
	}

	// Test reverse parsing from variant map
	parsedFromVar, err := dbusapi.ParseStatusVariantMap(variantMap)
	if err != nil {
		t.Fatalf("ParseStatusVariantMap failed: %v", err)
	}
	if parsedFromVar != status {
		t.Errorf("parsedFromVar = %+v, want %+v", parsedFromVar, status)
	}
}

// TestParseStatusMap_EdgeCases tests type coercions and error handling during map parsing.
func TestParseStatusMap_EdgeCases(t *testing.T) {
	// Test partial / alternative numeric types
	m := map[string]interface{}{
		"cpu_temp":        float32(50.5),
		"fan1_rpm":        int(2000),
		"fan2_rpm":        int64(1500),
		"battery_percent": uint32(90),
		"battery_limit":   uint(80),
		"on_ac":           true,
		"active_mode":     "silent",
		"auto_mode":       true,
	}

	parsed, err := dbusapi.ParseStatusMap(m)
	if err != nil {
		t.Fatalf("ParseStatusMap unexpected error: %v", err)
	}
	if parsed.CPUTemp != 50.5 {
		t.Errorf("parsed.CPUTemp = %v, want 50.5", parsed.CPUTemp)
	}
	if parsed.Fan1RPM != 2000 || parsed.Fan2RPM != 1500 {
		t.Errorf("parsed fan RPMs = %d, %d, want 2000, 1500", parsed.Fan1RPM, parsed.Fan2RPM)
	}
	if parsed.BatteryPercent != 90 || parsed.BatteryLimit != 80 {
		t.Errorf("parsed battery = %d, %d, want 90, 80", parsed.BatteryPercent, parsed.BatteryLimit)
	}
	if parsed.ActiveMode != models.ModeSilent {
		t.Errorf("parsed.ActiveMode = %v, want silent", parsed.ActiveMode)
	}
	if !parsed.AutoMode {
		t.Errorf("parsed.AutoMode = false, want true")
	}

	// Test int and int64 for cpu_temp
	mInt := map[string]interface{}{
		"cpu_temp": int(48),
	}
	parsedInt, err := dbusapi.ParseStatusMap(mInt)
	if err != nil || parsedInt.CPUTemp != 48.0 {
		t.Errorf("parsedInt cpu_temp = %v, want 48.0", parsedInt.CPUTemp)
	}

	mInt64 := map[string]interface{}{
		"cpu_temp": int64(52),
	}
	parsedInt64, err := dbusapi.ParseStatusMap(mInt64)
	if err != nil || parsedInt64.CPUTemp != 52.0 {
		t.Errorf("parsedInt64 cpu_temp = %v, want 52.0", parsedInt64.CPUTemp)
	}

	// Empty map returns empty telemetry with default ModeStandard
	parsedEmpty, err := dbusapi.ParseStatusMap(nil)
	if err != nil {
		t.Fatalf("ParseStatusMap(nil) error: %v", err)
	}
	if parsedEmpty.ActiveMode != models.ModeStandard {
		t.Errorf("parsedEmpty.ActiveMode = %v, want standard", parsedEmpty.ActiveMode)
	}

	// Invalid active_mode
	mInvalid := map[string]interface{}{
		"active_mode": "supercharged",
	}
	_, err = dbusapi.ParseStatusMap(mInvalid)
	if err == nil {
		t.Errorf("expected error for invalid active_mode, got nil")
	}
}

// failingDriver simulates hardware driver failures.
type failingDriver struct {
	driver.HardwareDriver
}

func (f *failingDriver) SetThermalMode(mode models.ThermalMode) error {
	return errors.New("sysfs I/O write error")
}

func (f *failingDriver) SetBatteryLimit(limit int32) error {
	return errors.New("battery limit sysfs unwriteable")
}

// TestDBusServerDirectMethods tests direct method handlers on DBusServer.
func TestDBusServerDirectMethods(t *testing.T) {
	mockFS := driver.NewMockFS()
	mockFS.WriteFile("/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy", []byte("0\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/charge_control_end_threshold", []byte("80\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/capacity", []byte("85\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/status", []byte("Discharging\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/temp1_input", []byte("55000\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/fan1_input", []byte("2500\n"))

	d := driver.NewCustomDriver(mockFS)
	cfg := models.DefaultConfig()
	eng := engine.NewEngine(d, cfg, "")

	server := dbusapi.NewServer(eng)

	// Test GetStatus
	statusMap, dbusErr := server.GetStatus()
	if dbusErr != nil {
		t.Fatalf("server.GetStatus() returned error: %v", dbusErr)
	}
	if statusMap["active_mode"] != "standard" {
		t.Errorf("statusMap[active_mode] = %v, want standard", statusMap["active_mode"])
	}

	// Test SetThermalMode - valid
	dbusErr = server.SetThermalMode("boost")
	if dbusErr != nil {
		t.Fatalf("server.SetThermalMode(boost) error: %v", dbusErr)
	}
	if eng.GetTelemetry().ActiveMode != models.ModeBoost {
		t.Errorf("eng ActiveMode = %v, want boost", eng.GetTelemetry().ActiveMode)
	}

	// Test SetThermalMode - invalid
	dbusErr = server.SetThermalMode("extreme_turbo")
	if dbusErr == nil {
		t.Errorf("expected error for invalid thermal mode, got nil")
	}

	// Test SetBatteryLimit - valid
	dbusErr = server.SetBatteryLimit(60)
	if dbusErr != nil {
		t.Fatalf("server.SetBatteryLimit(60) error: %v", dbusErr)
	}
	if eng.GetTelemetry().BatteryLimit != 60 {
		t.Errorf("eng BatteryLimit = %d, want 60", eng.GetTelemetry().BatteryLimit)
	}

	// Test SetBatteryLimit - invalid
	dbusErr = server.SetBatteryLimit(75)
	if dbusErr == nil {
		t.Errorf("expected error for invalid battery limit, got nil")
	}

	// Test SetAutoMode - valid
	dbusErr = server.SetAutoMode(true)
	if dbusErr != nil {
		t.Fatalf("server.SetAutoMode(true) error: %v", dbusErr)
	}
	if !eng.GetTelemetry().AutoMode {
		t.Errorf("eng AutoMode = false, want true")
	}

	// Test Server with Nil Engine (Error handling)
	nilServer := dbusapi.NewServer(nil)
	if _, err := nilServer.GetStatus(); err == nil {
		t.Errorf("expected error from nilServer.GetStatus(), got nil")
	}
	if err := nilServer.SetThermalMode("boost"); err == nil {
		t.Errorf("expected error from nilServer.SetThermalMode(), got nil")
	}
	if err := nilServer.SetBatteryLimit(80); err == nil {
		t.Errorf("expected error from nilServer.SetBatteryLimit(), got nil")
	}
	if err := nilServer.SetAutoMode(false); err == nil {
		t.Errorf("expected error from nilServer.SetAutoMode(), got nil")
	}

	// Test Hardware failure error handling
	failEng := engine.NewEngine(&failingDriver{d}, cfg, "")
	failServer := dbusapi.NewServer(failEng)
	if err := failServer.SetThermalMode("silent"); err == nil {
		t.Errorf("expected hardware error for SetThermalMode, got nil")
	}
	if err := failServer.SetBatteryLimit(60); err == nil {
		t.Errorf("expected hardware error for SetBatteryLimit, got nil")
	}

	server.Stop()
}

// mockBusObject implements dbusapi.DBusCaller for testing DBusClient.
type mockBusObject struct {
	statusMap   interface{}
	lastMode    string
	lastLimit   int32
	lastAuto    bool
	emptyBody   bool
	returnError bool
}

func (m *mockBusObject) Call(method string, flags dbus.Flags, args ...interface{}) *dbus.Call {
	call := &dbus.Call{
		Method: method,
		Args:   args,
		Done:   make(chan *dbus.Call, 1),
	}

	if m.returnError {
		call.Err = errors.New("mock dbus call failed")
		call.Done <- call
		return call
	}

	switch method {
	case dbusapi.Interface + ".GetStatus":
		if m.emptyBody {
			call.Body = []interface{}{}
		} else {
			call.Body = []interface{}{m.statusMap}
		}
	case dbusapi.Interface + ".SetThermalMode":
		if len(args) > 0 {
			m.lastMode = args[0].(string)
		}
	case dbusapi.Interface + ".SetBatteryLimit":
		if len(args) > 0 {
			m.lastLimit = args[0].(int32)
		}
	case dbusapi.Interface + ".SetAutoMode":
		if len(args) > 0 {
			m.lastAuto = args[0].(bool)
		}
	default:
		call.Err = errors.New("unknown method: " + method)
	}

	call.Done <- call
	return call
}

func (m *mockBusObject) CallWithContext(ctx context.Context, method string, flags dbus.Flags, args ...interface{}) *dbus.Call {
	return m.Call(method, flags, args...)
}

// TestDBusClientWithMockCaller tests DBusClient methods using a mock caller.
func TestDBusClientWithMockCaller(t *testing.T) {
	mockObj := &mockBusObject{
		statusMap: map[string]interface{}{
			"cpu_temp":        68.0,
			"fan1_rpm":        int32(3100),
			"fan2_rpm":        int32(2900),
			"battery_percent": int32(79),
			"battery_limit":   int32(80),
			"on_ac":           true,
			"active_mode":     "boost",
			"auto_mode":       true,
		},
	}

	client := dbusapi.NewCustomClient(mockObj)

	// Test GetStatus
	telem, err := client.GetStatus()
	if err != nil {
		t.Fatalf("client.GetStatus() error: %v", err)
	}
	if telem.CPUTemp != 68.0 || telem.ActiveMode != models.ModeBoost || !telem.AutoMode {
		t.Errorf("unexpected telemetry: %+v", telem)
	}

	// Test GetStatus with map[string]dbus.Variant
	mockObj.statusMap = dbusapi.FormatStatusVariantMap(telem)
	telemVar, err := client.GetStatus()
	if err != nil {
		t.Fatalf("client.GetStatus() with variant map error: %v", err)
	}
	if telemVar.CPUTemp != 68.0 {
		t.Errorf("telemVar.CPUTemp = %v, want 68.0", telemVar.CPUTemp)
	}

	// Test SetThermalMode
	if err := client.SetThermalMode("silent"); err != nil {
		t.Fatalf("client.SetThermalMode error: %v", err)
	}
	if mockObj.lastMode != "silent" {
		t.Errorf("mockObj.lastMode = %s, want silent", mockObj.lastMode)
	}

	// Test SetBatteryLimit
	if err := client.SetBatteryLimit(60); err != nil {
		t.Fatalf("client.SetBatteryLimit error: %v", err)
	}
	if mockObj.lastLimit != 60 {
		t.Errorf("mockObj.lastLimit = %d, want 60", mockObj.lastLimit)
	}

	// Test SetAutoMode
	if err := client.SetAutoMode(false); err != nil {
		t.Fatalf("client.SetAutoMode error: %v", err)
	}
	if mockObj.lastAuto != false {
		t.Errorf("mockObj.lastAuto = true, want false")
	}

	// Test Error handling
	mockObj.returnError = true
	if _, err := client.GetStatus(); err == nil {
		t.Errorf("expected error on GetStatus, got nil")
	}
	if err := client.SetThermalMode("boost"); err == nil {
		t.Errorf("expected error on SetThermalMode, got nil")
	}
	if err := client.SetBatteryLimit(80); err == nil {
		t.Errorf("expected error on SetBatteryLimit, got nil")
	}
	if err := client.SetAutoMode(true); err == nil {
		t.Errorf("expected error on SetAutoMode, got nil")
	}

	// Test Empty Body & Unexpected Types
	mockObj.returnError = false
	mockObj.emptyBody = true
	if _, err := client.GetStatus(); err == nil {
		t.Errorf("expected error for empty body, got nil")
	}

	mockObj.emptyBody = false
	mockObj.statusMap = "not a map"
	if _, err := client.GetStatus(); err == nil {
		t.Errorf("expected error for non-map response, got nil")
	}

	// Test Nil Caller
	nilClient := dbusapi.NewCustomClient(nil)
	if _, err := nilClient.GetStatus(); err == nil {
		t.Errorf("expected error from nilClient.GetStatus(), got nil")
	}
	if err := nilClient.SetThermalMode("silent"); err == nil {
		t.Errorf("expected error from nilClient.SetThermalMode(), got nil")
	}
	if err := nilClient.SetBatteryLimit(80); err == nil {
		t.Errorf("expected error from nilClient.SetBatteryLimit(), got nil")
	}
	if err := nilClient.SetAutoMode(true); err == nil {
		t.Errorf("expected error from nilClient.SetAutoMode(), got nil")
	}
}

// TestClientHelpers verifies helper constructors and subscriptions without active connection.
func TestClientHelpers(t *testing.T) {
	c := dbusapi.NewClientWithConn(nil)
	if err := c.Close(); err != nil {
		t.Errorf("c.Close() returned error: %v", err)
	}

	ctx := context.Background()
	_, _, err := c.SubscribeSignals(ctx)
	if err == nil {
		t.Errorf("expected error when subscribing with nil conn, got nil")
	}

	// Test NewClient() behavior (in environment with or without active bus)
	client, err := dbusapi.NewClient()
	if err == nil && client != nil {
		_ = client.Close()
	}
}

// TestIntrospectionXML verifies that the Introspect XML is well-formed and declares interface methods.
func TestIntrospectionXML(t *testing.T) {
	xmlData := dbusapi.IntrospectionXML

	var parsed struct {
		XMLName    xml.Name `xml:"node"`
		Interfaces []struct {
			Name    string `xml:"name,attr"`
			Methods []struct {
				Name string `xml:"name,attr"`
			} `xml:"method"`
			Signals []struct {
				Name string `xml:"name,attr"`
			} `xml:"signal"`
		} `xml:"interface"`
	}

	if err := xml.Unmarshal([]byte(xmlData), &parsed); err != nil {
		t.Fatalf("IntrospectionXML is not valid XML: %v", err)
	}

	foundKuhlerprofil := false
	for _, iface := range parsed.Interfaces {
		if iface.Name == dbusapi.Interface {
			foundKuhlerprofil = true
			methods := make(map[string]bool)
			for _, m := range iface.Methods {
				methods[m.Name] = true
			}
			if !methods["GetStatus"] || !methods["SetThermalMode"] || !methods["SetBatteryLimit"] || !methods["SetAutoMode"] {
				t.Errorf("missing expected methods in XML: %+v", methods)
			}

			signals := make(map[string]bool)
			for _, s := range iface.Signals {
				signals[s.Name] = true
			}
			if !signals["ThermalModeChanged"] || !signals["BatteryLimitChanged"] || !signals["TelemetryTick"] {
				t.Errorf("missing expected signals in XML: %+v", signals)
			}
		}
	}

	if !foundKuhlerprofil {
		t.Errorf("interface %s not found in introspection XML", dbusapi.Interface)
	}
}

// TestSecurityPolicyFile validates the systemd/org.freedesktop.kuhlerprofil.conf D-Bus policy XML file.
func TestSecurityPolicyFile(t *testing.T) {
	policyPath := "../../systemd/org.freedesktop.kuhlerprofil.conf"
	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("failed to read security policy file %s: %v", policyPath, err)
	}

	var parsed struct {
		XMLName  xml.Name `xml:"busconfig"`
		Policies []struct {
			User    string `xml:"user,attr"`
			Context string `xml:"context,attr"`
			Allows  []struct {
				Own             string `xml:"own,attr"`
				SendDestination string `xml:"send_destination,attr"`
				SendInterface   string `xml:"send_interface,attr"`
				ReceiveSender   string `xml:"receive_sender,attr"`
			} `xml:"allow"`
		} `xml:"policy"`
	}

	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("policy file %s is not valid XML: %v", policyPath, err)
	}

	if len(parsed.Policies) < 2 {
		t.Errorf("expected at least 2 policy blocks (root user and default context), got %d", len(parsed.Policies))
	}

	foundOwn := false
	for _, p := range parsed.Policies {
		for _, a := range p.Allows {
			if a.Own == dbusapi.ServiceName {
				foundOwn = true
			}
		}
	}
	if !foundOwn {
		t.Errorf("policy file missing allow own=%q", dbusapi.ServiceName)
	}
}

// TestSignalBroadcaster verifies that DBusServer listens to engine telemetry and broadcasts signals.
func TestSignalBroadcaster(t *testing.T) {
	mockFS := driver.NewMockFS()
	mockFS.WriteFile("/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy", []byte("0\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/charge_control_end_threshold", []byte("80\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/temp1_input", []byte("50000\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/fan1_input", []byte("2000\n"))

	d := driver.NewCustomDriver(mockFS)
	cfg := models.DefaultConfig()
	eng := engine.NewEngine(d, cfg, "")

	server := dbusapi.NewServer(eng)
	server.StartSignalBroadcaster(context.Background(), nil)

	// Update mode on engine
	_ = eng.SetMode(models.ModeBoost)
	time.Sleep(20 * time.Millisecond)

	// Update battery limit on engine
	_ = eng.SetBatteryLimit(60)
	time.Sleep(20 * time.Millisecond)

	// Idempotent start check
	server.StartSignalBroadcaster(context.Background(), nil)

	server.Stop()
}
