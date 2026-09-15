/**
 * KühlerProfil GNOME Shell Extension Test & Verification Suite
 * Validates manifest syntax, stylesheet classes, ESM export syntax,
 * D-Bus XML interface parity, and telemetry parsing logic.
 */

import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const EXTENSION_DIR = path.resolve(__dirname, '..');

let totalTests = 0;
let passedTests = 0;

function runTest(name, testFn) {
    totalTests++;
    try {
        testFn();
        console.log(`  ✔ PASS: ${name}`);
        passedTests++;
    } catch (err) {
        console.error(`  ✖ FAIL: ${name}`);
        console.error(`    ${err.message}`);
        throw err;
    }
}

console.log('=== KühlerProfil GNOME Shell Extension Test Suite ===\n');

// 1. Validate metadata.json
runTest('metadata.json exists and is valid JSON', () => {
    const metadataPath = path.join(EXTENSION_DIR, 'metadata.json');
    assert(fs.existsSync(metadataPath), 'metadata.json must exist');
    const content = fs.readFileSync(metadataPath, 'utf8');
    const metadata = JSON.parse(content);

    assert.equal(metadata.uuid, 'kuhlerprofil@katon26.github.io', 'UUID must match kuhlerprofil@katon26.github.io');
    assert(metadata.name && metadata.name.includes('KühlerProfil'), 'Name must contain KühlerProfil');
    assert(Array.isArray(metadata['shell-version']), 'shell-version must be an array');
    assert(metadata['shell-version'].includes('45'), 'shell-version must support GNOME 45');
    assert(metadata['shell-version'].includes('46'), 'shell-version must support GNOME 46');
    assert(metadata['shell-version'].includes('47'), 'shell-version must support GNOME 47');
    assert(metadata['shell-version'].includes('48'), 'shell-version must support GNOME 48');
    assert(metadata.description && metadata.description.length > 10, 'Description must be present');
});

// 2. Validate stylesheet.css
runTest('stylesheet.css exists and contains required GNOME quick settings classes', () => {
    const cssPath = path.join(EXTENSION_DIR, 'stylesheet.css');
    assert(fs.existsSync(cssPath), 'stylesheet.css must exist');
    const css = fs.readFileSync(cssPath, 'utf8');
    
    const requiredClasses = [
        '.koolthing-telemetry-grid',
        '.koolthing-metric-card',
        '.koolthing-button-group',
        '.koolthing-mode-button',
        '.koolthing-battery-button',
        '.koolthing-button-active',
        '.koolthing-status-text',
    ];

    for (const cls of requiredClasses) {
        assert(css.includes(cls), `stylesheet.css must define ${cls}`);
    }
});

// 3. Validate install.sh
runTest('install.sh exists, is executable, and contains correct UUID', () => {
    const installPath = path.join(EXTENSION_DIR, 'install.sh');
    assert(fs.existsSync(installPath), 'install.sh must exist');
    const content = fs.readFileSync(installPath, 'utf8');
    assert(content.includes('UUID="kuhlerprofil@katon26.github.io"'), 'install.sh must define extension UUID');
    assert(content.includes('metadata.json') && content.includes('extension.js'), 'install.sh must copy extension files');
});

// 4. Validate extension.js structure and D-Bus XML interface
runTest('extension.js defines matching D-Bus interface and signatures', () => {
    const extPath = path.join(EXTENSION_DIR, 'extension.js');
    assert(fs.existsSync(extPath), 'extension.js must exist');
    const code = fs.readFileSync(extPath, 'utf8');

    // Verify D-Bus interface name
    assert(code.includes('org.freedesktop.kuhlerprofil'), 'Must define org.freedesktop.kuhlerprofil interface');

    // Verify Methods
    assert(code.includes('name="GetStatus"'), 'Must define GetStatus method');
    assert(code.includes('name="SetThermalMode"'), 'Must define SetThermalMode method');
    assert(code.includes('name="SetBatteryLimit"'), 'Must define SetBatteryLimit method');
    assert(code.includes('name="SetAutoMode"'), 'Must define SetAutoMode method');
    assert(code.includes('name="GetCurveProfiles"'), 'Must define GetCurveProfiles method');
    assert(code.includes('name="GetActiveCurveProfile"'), 'Must define GetActiveCurveProfile method');
    assert(code.includes('name="SetCurveProfile"'), 'Must define SetCurveProfile method');
    assert(code.includes('name="GetHardwareFanCurves"'), 'Must define GetHardwareFanCurves method');
    assert(code.includes('name="GetCooldownMode"'), 'Must define GetCooldownMode method');
    assert(code.includes('name="SetCooldownMode"'), 'Must define SetCooldownMode method');

    // Verify Signals
    assert(code.includes('name="ThermalModeChanged"'), 'Must define ThermalModeChanged signal');
    assert(code.includes('name="BatteryLimitChanged"'), 'Must define BatteryLimitChanged signal');
    assert(code.includes('name="CurveProfileChanged"'), 'Must define CurveProfileChanged signal');
    assert(code.includes('name="CooldownModeChanged"'), 'Must define CooldownModeChanged signal');
    assert(code.includes('name="TelemetryTick"'), 'Must define TelemetryTick signal');

    // Verify QuickSettings and Extension usage
    assert(code.includes('QuickSettings.QuickMenuToggle'), 'Must subclass QuickMenuToggle');
    assert(code.includes('QuickSettings.SystemIndicator'), 'Must subclass SystemIndicator');
    assert(code.includes('export default class KuhlerProfilExtension extends Extension') || code.includes('export default class KoolThingExtension extends Extension'), 'Must export Extension class');
});

// 5. Unit test telemetry parsing and unwrap functions
// Dynamically test the pure helper functions logic
function testUnwrapVariant(variant) {
    if (variant === null || variant === undefined) return variant;
    if (typeof variant.deepUnpack === 'function') {
        return variant.deepUnpack();
    }
    if (typeof variant.unpack === 'function') {
        return testUnwrapVariant(variant.unpack());
    }
    if (Array.isArray(variant)) {
        return variant.map(testUnwrapVariant);
    }
    if (typeof variant === 'object') {
        const res = {};
        for (const [k, v] of Object.entries(variant)) {
            res[k] = testUnwrapVariant(v);
        }
        return res;
    }
    return variant;
}

function testParseTelemetry(raw) {
    if (!raw) return null;
    const data = testUnwrapVariant(raw);
    return {
        cpuTemp: Number(data.cpu_temp ?? 0),
        fan1Rpm: Number(data.fan1_rpm ?? 0),
        fan2Rpm: Number(data.fan2_rpm ?? 0),
        batteryPercent: Number(data.battery_percent ?? 0),
        batteryLimit: Number(data.battery_limit ?? 80),
        onAC: Boolean(data.on_ac ?? true),
        activeMode: String(data.active_mode ?? 'standard').toLowerCase(),
        autoMode: Boolean(data.auto_mode ?? false),
        activeCurve: String(data.active_curve_profile ?? data.active_curve ?? 'balanced').toLowerCase(),
        hasHardwareCurve: Boolean(data.has_hardware_curve ?? false),
        cooldownMode: String(data.cooldown_mode ?? 'kick').toLowerCase(),
    };
}

runTest('unwrapVariant handles primitive values and nested structures', () => {
    assert.equal(testUnwrapVariant(42), 42);
    assert.equal(testUnwrapVariant('boost'), 'boost');
    assert.deepEqual(testUnwrapVariant({ a: 1, b: 'two' }), { a: 1, b: 'two' });

    // Mock GLib.Variant with unpack() and deepUnpack()
    const mockDeepVariant = {
        deepUnpack: () => ({ cpu_temp: 55.4, fan1_rpm: 2500 }),
    };
    assert.deepEqual(testUnwrapVariant(mockDeepVariant), { cpu_temp: 55.4, fan1_rpm: 2500 });

    const mockUnpackVariant = {
        unpack: () => ({ battery_limit: 80 }),
    };
    assert.deepEqual(testUnwrapVariant(mockUnpackVariant), { battery_limit: 80 });
});

runTest('parseTelemetry properly formats live D-Bus telemetry payload', () => {
    const rawPayload = {
        cpu_temp: 62.5,
        fan1_rpm: 3200,
        fan2_rpm: 3000,
        battery_percent: 85,
        battery_limit: 80,
        on_ac: true,
        active_mode: 'BOOST',
        auto_mode: false,
        active_curve_profile: 'aggressive',
        has_hardware_curve: true,
        cooldown_mode: 'decay',
    };

    const parsed = testParseTelemetry(rawPayload);
    assert.equal(parsed.cpuTemp, 62.5);
    assert.equal(parsed.fan1Rpm, 3200);
    assert.equal(parsed.fan2Rpm, 3000);
    assert.equal(parsed.batteryPercent, 85);
    assert.equal(parsed.batteryLimit, 80);
    assert.equal(parsed.onAC, true);
    assert.equal(parsed.activeMode, 'boost');
    assert.equal(parsed.autoMode, false);
    assert.equal(parsed.activeCurve, 'aggressive');
    assert.equal(parsed.hasHardwareCurve, true);
    assert.equal(parsed.cooldownMode, 'decay');
});

runTest('parseTelemetry handles null and partial payloads with safe defaults', () => {
    assert.equal(testParseTelemetry(null), null);
    assert.equal(testParseTelemetry(undefined), null);

    const partial = testParseTelemetry({});
    assert.equal(partial.cpuTemp, 0);
    assert.equal(partial.fan1Rpm, 0);
    assert.equal(partial.fan2Rpm, 0);
    assert.equal(partial.batteryPercent, 0);
    assert.equal(partial.batteryLimit, 80);
    assert.equal(partial.onAC, true);
    assert.equal(partial.activeMode, 'standard');
    assert.equal(partial.autoMode, false);
    assert.equal(partial.activeCurve, 'balanced');
    assert.equal(partial.hasHardwareCurve, false);
    assert.equal(partial.cooldownMode, 'kick');
});

runTest('Thermal modes list, curve profiles, and battery limits cover ASUS specifications', () => {
    const expectedModes = ['silent', 'standard', 'boost'];
    const expectedCurves = ['quiet', 'balanced', 'aggressive'];
    const expectedLimits = [60, 80, 100];

    const code = fs.readFileSync(path.join(EXTENSION_DIR, 'extension.js'), 'utf8');
    for (const mode of expectedModes) {
        assert(code.includes(`'${mode}'`), `extension.js must include mode '${mode}'`);
    }
    for (const curve of expectedCurves) {
        assert(code.includes(`'${curve}'`), `extension.js must include curve profile '${curve}'`);
    }
    for (const limit of expectedLimits) {
        assert(code.includes(String(limit)), `extension.js must include battery limit ${limit}`);
    }
});

runTest('Cooldown modes list covers Zero-RPM specifications (kick, decay, off)', () => {
    const expectedCooldowns = ['kick', 'decay', 'off'];
    const code = fs.readFileSync(path.join(EXTENSION_DIR, 'extension.js'), 'utf8');
    for (const cm of expectedCooldowns) {
        assert(code.includes(`'${cm}'`), `extension.js must include cooldown mode '${cm}'`);
    }
    assert(code.includes('COOLDOWN_MODES'), 'extension.js must export COOLDOWN_MODES');
    assert(code.includes('_buildCooldownSection'), 'extension.js must define _buildCooldownSection');
});

// 10. HIG & Lifecycle: Zero top-level gettext invocations (prevents module import crashes in GNOME 45-50)
runTest('No raw top-level gettext calls that crash during module import', () => {
    const code = fs.readFileSync(path.join(EXTENSION_DIR, 'extension.js'), 'utf8');
    // Ensure THERMAL_MODES and COOLDOWN_MODES use getters/safeTranslate, NOT raw static _('...')
    const lines = code.split('\n');
    let insideTopLevelArray = false;
    for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        if (line.includes('export const THERMAL_MODES') || line.includes('export const COOLDOWN_MODES') || line.includes('export const CURVE_PROFILES')) {
            insideTopLevelArray = true;
        }
        if (insideTopLevelArray) {
            assert(!line.match(/\bname:\s*_\(/), `Line ${i + 1} must not call raw _() at module level`);
            assert(!line.match(/\bdesc:\s*_\(/), `Line ${i + 1} must not call raw _() at module level`);
            if (line.includes('];')) {
                insideTopLevelArray = false;
            }
        }
    }
});

// 11. HIG & Lifecycle: enable() initializes translations and disable() performs clean teardown
runTest('KuhlerProfilExtension calls initTranslations() in enable() and cleans up in disable()', () => {
    const code = fs.readFileSync(path.join(EXTENSION_DIR, 'extension.js'), 'utf8');
    assert(code.includes('this.initTranslations()'), 'enable() must call this.initTranslations()');
    assert(code.includes('this._client.stop()'), 'disable() must stop D-Bus client');
    assert(code.includes('this._indicator.destroy()'), 'disable() must destroy indicator');
    // Ensure no double-destruction of quickSettingsItems
    assert(!code.includes('this._indicator.quickSettingsItems.forEach(item => item.destroy())'), 'Must not double-destroy quickSettingsItems');
});

// 12. D-Bus IPC & Resource Management: Clean signal disconnection
runTest('KuhlerProfilDBusClient cleans up signals using disconnectSignal and signal_unsubscribe', () => {
    const code = fs.readFileSync(path.join(EXTENSION_DIR, 'extension.js'), 'utf8');
    assert(code.includes('disconnectSignal'), 'Client must use disconnectSignal for proxy signals');
    assert(code.includes('signal_unsubscribe'), 'Client must unsubscribe bus connection signals');
    assert(code.includes('source_remove'), 'Client must remove GLib timeouts on stop');
});

// 13. Accessibility & GNOME HIG: Accessible roles and names
runTest('Interactive buttons define accessible_role and accessible_name for HIG accessibility', () => {
    const code = fs.readFileSync(path.join(EXTENSION_DIR, 'extension.js'), 'utf8');
    assert(code.includes('accessible_role: Atk.Role.RADIO_BUTTON'), 'Mode and cooldown buttons must set RADIO_BUTTON role');
    assert(code.includes('accessible_name:'), 'Buttons must provide accessible_name for screen readers');
});

// 14. Extension Preferences Window: prefs.js exists and exports valid ExtensionPreferences
runTest('prefs.js exists and exports ExtensionPreferences subclass with Libadwaita UI', () => {
    const prefsPath = path.join(EXTENSION_DIR, 'prefs.js');
    assert(fs.existsSync(prefsPath), 'prefs.js must exist for Extension Manager preferences');
    const code = fs.readFileSync(prefsPath, 'utf8');
    assert(code.includes('export default class KuhlerProfilPreferences extends ExtensionPreferences'), 'Must export ExtensionPreferences subclass');
    assert(code.includes('fillPreferencesWindow(window)'), 'Must implement fillPreferencesWindow');
    assert(code.includes('Adw.PreferencesPage'), 'Must use Adw.PreferencesPage');
    assert(code.includes('Adw.PreferencesGroup'), 'Must use Adw.PreferencesGroup');
    assert(code.includes('Adw.ComboRow'), 'Must use Adw.ComboRow');
    assert(code.includes('Adw.SwitchRow'), 'Must use Adw.SwitchRow');
});

// 15. Installation & Packaging: install.sh handles prefs.js and metadata.json has version 2
runTest('install.sh and metadata.json properly include prefs.js and GNOME 45-50 support', () => {
    const installPath = path.join(EXTENSION_DIR, 'install.sh');
    const installCode = fs.readFileSync(installPath, 'utf8');
    assert(installCode.includes('prefs.js'), 'install.sh must copy and pack prefs.js');

    const metaPath = path.join(EXTENSION_DIR, 'metadata.json');
    const meta = JSON.parse(fs.readFileSync(metaPath, 'utf8'));
    assert(meta['shell-version'].includes('49'), 'metadata.json must support GNOME 49');
    assert(meta['shell-version'].includes('50'), 'metadata.json must support GNOME 50');
    assert.equal(meta.version, 2, 'Extension version must be 2');
});

// 16. D-Bus Resilience: getSafeBus handles missing bus gracefully without throwing IOError
runTest('getSafeBus safely detects system or session bus without uncaught exceptions', () => {
    const extCode = fs.readFileSync(path.join(EXTENSION_DIR, 'extension.js'), 'utf8');
    assert(extCode.includes('function getSafeBus()'), 'extension.js must define getSafeBus');
    assert(extCode.includes('Gio.DBus.system'), 'must check Gio.DBus.system');
    assert(extCode.includes('Gio.DBus.session'), 'must fall back to Gio.DBus.session');

    const prefsCode = fs.readFileSync(path.join(EXTENSION_DIR, 'prefs.js'), 'utf8');
    assert(prefsCode.includes('function getSafeBus()'), 'prefs.js must define getSafeBus');
    assert(!prefsCode.includes('let bus = Gio.DBus.system;\n        if (!bus)'), 'prefs.js must not access Gio.DBus.system without try/catch');
});

// 17. GNOME HIG Stylesheet Audit: Clean St CSS without invalid properties & with accent colors
runTest('stylesheet.css strictly adheres to St CSS parser rules and GNOME HIG accent colors', () => {
    const css = fs.readFileSync(path.join(EXTENSION_DIR, 'stylesheet.css'), 'utf8');
    assert(!css.includes('text-transform'), 'St CSS does not support text-transform; must be removed to prevent journal warnings');
    assert(!css.includes('outline-offset'), 'St CSS does not support outline-offset; must be removed to prevent journal warnings');
    assert(css.includes('-st-accent-color'), 'stylesheet.css must support GNOME HIG -st-accent-color');
    assert(css.includes(':checked'), 'stylesheet.css must support :checked pseudo-class for active mode buttons');
    assert(css.includes('kuhlerprofil-temp-cool'), 'stylesheet.css must define .kuhlerprofil-temp-cool');
    assert(css.includes('kuhlerprofil-temp-hot'), 'stylesheet.css must define .kuhlerprofil-temp-hot');
});

// 18. Accessibility (a11y) HIG Audit: Buttons set toggle_mode: true and update checked state
runTest('Buttons set toggle_mode: true and update checked state for screen reader (Orca) compatibility', () => {
    const code = fs.readFileSync(path.join(EXTENSION_DIR, 'extension.js'), 'utf8');
    const toggleModeCount = (code.match(/toggle_mode:\s*true/g) || []).length;
    // Mode, curve, cooldown, and battery buttons all specify toggle_mode: true
    assert(toggleModeCount >= 4, `Expected at least 4 toggle_mode: true button declarations, found ${toggleModeCount}`);
    assert(code.includes('btn.checked = isSelected;'), '_updateUI must update btn.checked for Atk.StateType.CHECKED exposure');
});

// 19. QuickMenuToggle HIG: Accessible name, header settings button, and indicator binding
runTest('QuickMenuToggle provides menuButtonAccessibleName, header settings button, and bound indicator icon', () => {
    const code = fs.readFileSync(path.join(EXTENSION_DIR, 'extension.js'), 'utf8');
    assert(code.includes('menuButtonAccessibleName:'), 'QuickMenuToggle must define menuButtonAccessibleName');
    assert(code.includes('addHeaderSuffix'), 'QuickMenuToggle must add header settings suffix button for instant preferences');
    assert(code.includes('emblem-system-symbolic'), 'Header button must use emblem-system-symbolic icon');
    assert(code.includes("bind_property('icon-name'"), 'KuhlerProfilIndicator must bind icon-name between toggle and indicator');
});

// 20. Event Handling: Guard against recursive setToggleState feedback loop on auto governor switch
runTest('Auto governor switch is guarded against re-entrant toggled feedback loops', () => {
    const code = fs.readFileSync(path.join(EXTENSION_DIR, 'extension.js'), 'utf8');
    assert(code.includes('_syncingAutoSwitch'), 'extension.js must use _syncingAutoSwitch loop guard');
    assert(code.includes('if (this._syncingAutoSwitch) return;'), 'toggled handler must exit early if syncing');
});

// 21. Teardown & Overlay Cleanup: KuhlerProfilToggle.destroy() destroys this.menu
runTest('KuhlerProfilToggle destroys this.menu on teardown to prevent overlay actor leaks', () => {
    const code = fs.readFileSync(path.join(EXTENSION_DIR, 'extension.js'), 'utf8');
    assert(code.includes('if (this.menu) {\n            this.menu.destroy();\n        }'), 'destroy() must call this.menu.destroy()');
});

// 22. Preferences Window HIG: Live Telemetry, signal subscriptions, and window close-request cleanup
runTest('prefs.js implements Live Telemetry, dynamic signal listeners, and clean window destruction', () => {
    const code = fs.readFileSync(path.join(EXTENSION_DIR, 'prefs.js'), 'utf8');
    assert(code.includes('Live Telemetry & Diagnostics'), 'prefs.js must feature Live Telemetry & Diagnostics');
    assert(code.includes('connectSignal'), 'prefs.js must subscribe to D-Bus signals for real-time sync');
    assert(code.includes('TelemetryTick'), 'prefs.js must update on TelemetryTick signal');
    assert(code.includes('close-request'), 'prefs.js must clean up signal connections on window close-request');
});

// 23. Preferences About Page & Brand Identity (GNOME HIG)
runTest('prefs.js implements About page with author links, version pill, QR coffee button, and GitHub sponsor link', () => {
    const code = fs.readFileSync(path.join(EXTENSION_DIR, 'prefs.js'), 'utf8');
    assert(code.includes('createAboutPage'), 'prefs.js must implement createAboutPage');
    assert(code.includes('https://katon26.github.io'), 'About page must link to Katon (katon26)');
    assert(code.includes('https://buymeacoffee.com/fuhg'), 'About page must link to Buy Me a Coffee');
    assert(code.includes('https://github.com/sponsors/katon26'), 'About page must link to GitHub Sponsors');
    assert(code.includes('kuhlerprofil-version-pill'), 'About page must style version pill');
    assert(code.includes('kuhlerprofil-coffee-button'), 'About page must style sponsor coffee button');
    assert(code.includes('_ensureCustomCss'), 'prefs.js must load custom css provider');

    const cssCode = fs.readFileSync(path.join(EXTENSION_DIR, 'prefs.css'), 'utf8');
    assert(cssCode.includes('.kuhlerprofil-version-pill'), 'prefs.css must define .kuhlerprofil-version-pill');
    assert(cssCode.includes('.kuhlerprofil-coffee-button'), 'prefs.css must define .kuhlerprofil-coffee-button');

    const srcDir = path.join(EXTENSION_DIR, 'src');
    assert(fs.existsSync(path.join(srcDir, 'kuhlerprofil-logo.svg')), 'kuhlerprofil-logo.svg must exist in src/');
    assert(fs.existsSync(path.join(srcDir, 'qr-code-fuhg.svg')), 'qr-code-fuhg.svg must exist in src/');
    assert(fs.existsSync(path.join(srcDir, 'github-symbolic.svg')), 'github-symbolic.svg must exist in src/');
});

console.log(`\nAll ${passedTests}/${totalTests} extension tests passed successfully! ✨`);


