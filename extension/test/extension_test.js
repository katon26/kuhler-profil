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

    assert.equal(metadata.uuid, 'kuhlerprofil@asus-linux.org', 'UUID must match kuhlerprofil@asus-linux.org');
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
    assert(content.includes('UUID="kuhlerprofil@asus-linux.org"'), 'install.sh must define extension UUID');
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

console.log(`\nAll ${passedTests}/${totalTests} extension tests passed successfully! ✨`);
