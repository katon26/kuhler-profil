/**
 * KühlerProfil GNOME Shell Quick Settings Extension
 * Provides real-time thermal monitoring, fan RPM telemetry, ASUS thermal profile
 * switching, dynamic fan curves, zero-RPM cooldown assistance, and battery health
 * charge threshold control over Linux D-Bus IPC.
 *
 * Supported GNOME Shell Versions: 45, 46, 47, 48, 49, 50
 */

import Atk from 'gi://Atk';
import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import GObject from 'gi://GObject';
import St from 'gi://St';
import Clutter from 'gi://Clutter';

import { Extension, gettext as _ } from 'resource:///org/gnome/shell/extensions/extension.js';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import * as QuickSettings from 'resource:///org/gnome/shell/ui/quickSettings.js';
import * as PopupMenu from 'resource:///org/gnome/shell/ui/popupMenu.js';

export const DBUS_NAME = 'org.freedesktop.kuhlerprofil';
export const DBUS_PATH = '/org/freedesktop/kuhlerprofil';
export const DBUS_INTERFACE = 'org.freedesktop.kuhlerprofil';

export const KuhlerProfilInterfaceXML = `
<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN"
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
      <arg name="profile" type="s" direction="out"/>
    </method>
    <method name="SetCurveProfile">
      <arg name="profile" type="s" direction="in"/>
    </method>
    <method name="GetHardwareFanCurves">
      <arg name="status" type="a{sv}" direction="out"/>
    </method>
    <method name="GetCooldownMode">
      <arg name="mode" type="s" direction="out"/>
    </method>
    <method name="SetCooldownMode">
      <arg name="mode" type="s" direction="in"/>
    </method>
    <signal name="ThermalModeChanged">
      <arg name="mode" type="s"/>
    </signal>
    <signal name="BatteryLimitChanged">
      <arg name="limit" type="i"/>
    </signal>
    <signal name="CurveProfileChanged">
      <arg name="profile" type="s"/>
    </signal>
    <signal name="CooldownModeChanged">
      <arg name="mode" type="s"/>
    </signal>
    <signal name="TelemetryTick">
      <arg name="status" type="a{sv}"/>
    </signal>
  </interface>
</node>`;

export const KoolThingInterfaceXML = KuhlerProfilInterfaceXML;

/**
 * Safe translation wrapper that falls back gracefully to English
 * if called before gettext is initialized.
 */
function safeTranslate(str) {
    try {
        if (typeof _ === 'function') {
            return _(str);
        }
    } catch (_) {}
    return str;
}

/**
 * Safely resolves the system or session D-Bus connection without
 * throwing unhandled IOErrors if a bus socket is absent.
 */
export function getSafeBus() {
    try {
        if (Gio.DBus.system) return Gio.DBus.system;
    } catch (_) {}
    try {
        if (Gio.DBus.session) return Gio.DBus.session;
    } catch (_) {}
    return null;
}

/**
 * Thermal modes, cooldown options, and curve profiles.
 * Names and descriptions use dynamic getters so no gettext calls execute at module import.
 */
export const THERMAL_MODES = [
    { id: 'silent', get name() { return safeTranslate('Silent'); }, icon: 'power-profile-power-saver-symbolic', get desc() { return safeTranslate('Quiet'); } },
    { id: 'standard', get name() { return safeTranslate('Standard'); }, icon: 'power-profile-balanced-symbolic', get desc() { return safeTranslate('Balanced'); } },
    { id: 'boost', get name() { return safeTranslate('Boost'); }, icon: 'power-profile-performance-symbolic', get desc() { return safeTranslate('Max Cooling'); } },
];

export const COOLDOWN_MODES = [
    { id: 'kick', get name() { return safeTranslate('Kick'); }, get desc() { return safeTranslate('Instant Reset'); } },
    { id: 'decay', get name() { return safeTranslate('Decay'); }, get desc() { return safeTranslate('15s Smooth'); } },
    { id: 'off', get name() { return safeTranslate('Off'); }, get desc() { return safeTranslate('Factory'); } },
];

export const CURVE_PROFILES = [
    { id: 'quiet', get name() { return safeTranslate('Quiet'); }, get desc() { return safeTranslate('Acoustic Priority'); } },
    { id: 'balanced', get name() { return safeTranslate('Balanced'); }, get desc() { return safeTranslate('Dynamic Everyday'); } },
    { id: 'aggressive', get name() { return safeTranslate('Aggressive'); }, get desc() { return safeTranslate('Maximum Cooling'); } },
];

export const BATTERY_LIMITS = [60, 80, 100];

/**
 * Unwraps GLib Variants or nested object structures into plain JavaScript types.
 */
export function unwrapVariant(variant) {
    if (variant === null || variant === undefined) return variant;
    if (typeof variant.deepUnpack === 'function') {
        return variant.deepUnpack();
    }
    if (typeof variant.unpack === 'function') {
        return unwrapVariant(variant.unpack());
    }
    if (Array.isArray(variant)) {
        return variant.map(unwrapVariant);
    }
    if (typeof variant === 'object') {
        const res = {};
        for (const [k, v] of Object.entries(variant)) {
            res[k] = unwrapVariant(v);
        }
        return res;
    }
    return variant;
}

/**
 * Parses and sanitizes a raw telemetry dictionary from D-Bus into typed metrics.
 */
export function parseTelemetry(raw) {
    if (!raw) return null;
    const data = unwrapVariant(raw);
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

/**
 * Formats a thermal mode ID into display text and matching symbolic icon.
 */
export function getModeInfo(mode) {
    const normalized = String(mode || 'standard').toLowerCase();
    const found = THERMAL_MODES.find(m => m.id === normalized);
    if (found) return found;
    return {
        id: normalized,
        name: normalized.charAt(0).toUpperCase() + normalized.slice(1),
        icon: 'power-profile-balanced-symbolic',
        desc: '',
    };
}

/**
 * Robust D-Bus client wrapper for org.freedesktop.kuhlerprofil.
 */
export class KuhlerProfilDBusClient {
    constructor() {
        this._proxy = null;
        this._subscribers = new Set();
        this._signalIds = [];
        this._busSignalId = 0;
        this._pollTimerId = 0;
        this._connection = null;
        this._connected = false;
        this._lastTelemetry = null;
    }

    start() {
        this._initProxy();
        // Poll every 3 seconds as watchdog for live metrics
        this._pollTimerId = GLib.timeout_add_seconds(GLib.PRIORITY_DEFAULT, 3, () => {
            if (this._connected && this._proxy) {
                this.getStatus().catch(() => {});
            } else if (!this._connected) {
                this._initProxy();
            }
            return GLib.SOURCE_CONTINUE;
        });
    }

    stop() {
        if (this._pollTimerId) {
            GLib.source_remove(this._pollTimerId);
            this._pollTimerId = 0;
        }
        this._cleanupSignals();
        this._proxy = null;
        this._connection = null;
        this._connected = false;
        this._subscribers.clear();
    }

    subscribe(callback) {
        this._subscribers.add(callback);
        if (this._lastTelemetry) {
            try {
                callback(this._lastTelemetry, this._connected);
            } catch (e) {
                console.error(`[KühlerProfil] Subscriber initial callback error: ${e}`);
            }
        }
        return () => this._subscribers.delete(callback);
    }

    _notify(telemetry) {
        if (telemetry) {
            this._lastTelemetry = telemetry;
        }
        for (const cb of this._subscribers) {
            try {
                cb(this._lastTelemetry, this._connected);
            } catch (e) {
                console.error(`[KühlerProfil] Error in subscriber callback: ${e}`);
            }
        }
    }

    _cleanupSignals() {
        if (this._proxy && this._signalIds.length > 0) {
            for (const id of this._signalIds) {
                try {
                    if (typeof this._proxy.disconnectSignal === 'function') {
                        this._proxy.disconnectSignal(id);
                    } else if (typeof this._proxy.disconnect === 'function') {
                        this._proxy.disconnect(id);
                    }
                } catch (_) {}
            }
            this._signalIds = [];
        }
        if (this._connection && this._busSignalId) {
            try {
                this._connection.signal_unsubscribe(this._busSignalId);
            } catch (_) {}
            this._busSignalId = 0;
        }
    }

    async _initProxy() {
        try {
            const KuhlerProfilProxyWrapper = Gio.DBusProxy.makeProxyWrapper(KuhlerProfilInterfaceXML);

            const bus = getSafeBus();
            if (!bus) {
                this._connected = false;
                this._notify(null);
                return;
            }
            this._connection = bus;

            this._proxy = new KuhlerProfilProxyWrapper(
                bus,
                DBUS_NAME,
                DBUS_PATH,
                (proxy, error) => {
                    if (error) {
                        this._connected = false;
                        this._notify(null);
                        return;
                    }
                    this._setupSignalListeners();
                    this.getStatus().catch(() => {});
                }
            );
        } catch (e) {
            this._connected = false;
            this._notify(null);
        }
    }

    _setupSignalListeners() {
        this._cleanupSignals();

        if (!this._proxy) return;

        try {
            const sigModeId = this._proxy.connectSignal('ThermalModeChanged', (_proxy, _sender, [mode]) => {
                if (this._lastTelemetry) {
                    this._lastTelemetry.activeMode = String(mode).toLowerCase();
                    this._notify(this._lastTelemetry);
                } else {
                    this.getStatus().catch(() => {});
                }
            });
            if (sigModeId) this._signalIds.push(sigModeId);

            const sigLimitId = this._proxy.connectSignal('BatteryLimitChanged', (_proxy, _sender, [limit]) => {
                if (this._lastTelemetry) {
                    this._lastTelemetry.batteryLimit = Number(limit);
                    this._notify(this._lastTelemetry);
                } else {
                    this.getStatus().catch(() => {});
                }
            });
            if (sigLimitId) this._signalIds.push(sigLimitId);

            const sigCurveId = this._proxy.connectSignal('CurveProfileChanged', (_proxy, _sender, [profile]) => {
                if (this._lastTelemetry) {
                    this._lastTelemetry.activeCurve = String(profile).toLowerCase();
                    this._notify(this._lastTelemetry);
                } else {
                    this.getStatus().catch(() => {});
                }
            });
            if (sigCurveId) this._signalIds.push(sigCurveId);

            const sigCooldownId = this._proxy.connectSignal('CooldownModeChanged', (_proxy, _sender, [mode]) => {
                if (this._lastTelemetry) {
                    this._lastTelemetry.cooldownMode = String(mode).toLowerCase();
                    this._notify(this._lastTelemetry);
                } else {
                    this.getStatus().catch(() => {});
                }
            });
            if (sigCooldownId) this._signalIds.push(sigCooldownId);

            const sigTelemId = this._proxy.connectSignal('TelemetryTick', (_proxy, _sender, [status]) => {
                const parsed = parseTelemetry(status);
                if (parsed) {
                    this._connected = true;
                    this._notify(parsed);
                }
            });
            if (sigTelemId) this._signalIds.push(sigTelemId);

            // Also listen via generic bus connection for robustness
            if (this._connection) {
                this._busSignalId = this._connection.signal_subscribe(
                    DBUS_NAME,
                    DBUS_INTERFACE,
                    null,
                    DBUS_PATH,
                    null,
                    Gio.DBusSignalFlags.NONE,
                    (_conn, _sender, _path, _iface, signalName, parameters) => {
                        this._onBusSignal(signalName, parameters);
                    }
                );
            }
        } catch (e) {
            console.error(`[KühlerProfil] Failed to bind signal listeners: ${e}`);
        }
    }

    _onBusSignal(signalName, parameters) {
        if (!parameters) return;
        try {
            const unwrapped = unwrapVariant(parameters);
            if (signalName === 'ThermalModeChanged') {
                const mode = Array.isArray(unwrapped) ? unwrapped[0] : unwrapped;
                if (this._lastTelemetry) {
                    this._lastTelemetry.activeMode = String(mode).toLowerCase();
                    this._connected = true;
                    this._notify(this._lastTelemetry);
                }
            } else if (signalName === 'BatteryLimitChanged') {
                const limit = Array.isArray(unwrapped) ? unwrapped[0] : unwrapped;
                if (this._lastTelemetry) {
                    this._lastTelemetry.batteryLimit = Number(limit);
                    this._connected = true;
                    this._notify(this._lastTelemetry);
                }
            } else if (signalName === 'CurveProfileChanged') {
                const profile = Array.isArray(unwrapped) ? unwrapped[0] : unwrapped;
                if (this._lastTelemetry) {
                    this._lastTelemetry.activeCurve = String(profile).toLowerCase();
                    this._connected = true;
                    this._notify(this._lastTelemetry);
                }
            } else if (signalName === 'CooldownModeChanged') {
                const mode = Array.isArray(unwrapped) ? unwrapped[0] : unwrapped;
                if (this._lastTelemetry) {
                    this._lastTelemetry.cooldownMode = String(mode).toLowerCase();
                    this._connected = true;
                    this._notify(this._lastTelemetry);
                }
            } else if (signalName === 'TelemetryTick') {
                const status = Array.isArray(unwrapped) ? unwrapped[0] : unwrapped;
                const parsed = parseTelemetry(status);
                if (parsed) {
                    this._connected = true;
                    this._notify(parsed);
                }
            }
        } catch (e) {
            console.error(`[KühlerProfil] Error processing bus signal ${signalName}: ${e}`);
        }
    }

    async getStatus() {
        if (!this._proxy) throw new Error('D-Bus proxy not initialized');

        return new Promise((resolve, reject) => {
            this._proxy.GetStatusRemote((result, error) => {
                if (error) {
                    this._connected = false;
                    this._notify(null);
                    reject(error);
                    return;
                }
                try {
                    const raw = Array.isArray(result) ? result[0] : result;
                    const parsed = parseTelemetry(raw);
                    this._connected = true;
                    this._notify(parsed);
                    resolve(parsed);
                } catch (e) {
                    reject(e);
                }
            });
        });
    }

    async setThermalMode(mode) {
        if (!this._proxy) throw new Error('D-Bus proxy not initialized');

        return new Promise((resolve, reject) => {
            this._proxy.SetThermalModeRemote(String(mode), (_result, error) => {
                if (error) {
                    reject(error);
                    return;
                }
                if (this._lastTelemetry) {
                    this._lastTelemetry.activeMode = String(mode).toLowerCase();
                    this._notify(this._lastTelemetry);
                }
                resolve();
            });
        });
    }

    async setBatteryLimit(limit) {
        if (!this._proxy) throw new Error('D-Bus proxy not initialized');

        return new Promise((resolve, reject) => {
            this._proxy.SetBatteryLimitRemote(Number(limit), (_result, error) => {
                if (error) {
                    reject(error);
                    return;
                }
                if (this._lastTelemetry) {
                    this._lastTelemetry.batteryLimit = Number(limit);
                    this._notify(this._lastTelemetry);
                }
                resolve();
            });
        });
    }

    async setAutoMode(enabled) {
        if (!this._proxy) throw new Error('D-Bus proxy not initialized');

        return new Promise((resolve, reject) => {
            this._proxy.SetAutoModeRemote(Boolean(enabled), (_result, error) => {
                if (error) {
                    reject(error);
                    return;
                }
                if (this._lastTelemetry) {
                    this._lastTelemetry.autoMode = Boolean(enabled);
                    this._notify(this._lastTelemetry);
                }
                resolve();
            });
        });
    }

    async getCurveProfiles() {
        if (!this._proxy) throw new Error('D-Bus proxy not initialized');

        return new Promise((resolve, reject) => {
            this._proxy.GetCurveProfilesRemote((result, error) => {
                if (error) {
                    reject(error);
                    return;
                }
                const unwrapped = unwrapVariant(result);
                const profiles = Array.isArray(unwrapped) ? unwrapped[0] : unwrapped;
                resolve(profiles);
            });
        });
    }

    async getActiveCurveProfile() {
        if (!this._proxy) throw new Error('D-Bus proxy not initialized');

        return new Promise((resolve, reject) => {
            this._proxy.GetActiveCurveProfileRemote((result, error) => {
                if (error) {
                    reject(error);
                    return;
                }
                const unwrapped = unwrapVariant(result);
                const active = Array.isArray(unwrapped) ? unwrapped[0] : unwrapped;
                resolve(String(active));
            });
        });
    }

    async setCurveProfile(profile) {
        if (!this._proxy) throw new Error('D-Bus proxy not initialized');

        return new Promise((resolve, reject) => {
            this._proxy.SetCurveProfileRemote(String(profile), (_result, error) => {
                if (error) {
                    reject(error);
                    return;
                }
                if (this._lastTelemetry) {
                    this._lastTelemetry.activeCurve = String(profile).toLowerCase();
                    this._notify(this._lastTelemetry);
                }
                resolve();
            });
        });
    }

    async getCooldownMode() {
        if (!this._proxy) throw new Error('D-Bus proxy not initialized');

        return new Promise((resolve, reject) => {
            this._proxy.GetCooldownModeRemote((result, error) => {
                if (error) {
                    reject(error);
                    return;
                }
                const unwrapped = unwrapVariant(result);
                const mode = Array.isArray(unwrapped) ? unwrapped[0] : unwrapped;
                resolve(String(mode));
            });
        });
    }

    async setCooldownMode(mode) {
        if (!this._proxy) throw new Error('D-Bus proxy not initialized');

        return new Promise((resolve, reject) => {
            this._proxy.SetCooldownModeRemote(String(mode), (_result, error) => {
                if (error) {
                    reject(error);
                    return;
                }
                if (this._lastTelemetry) {
                    this._lastTelemetry.cooldownMode = String(mode).toLowerCase();
                    this._notify(this._lastTelemetry);
                }
                resolve();
            });
        });
    }
}

export const KoolThingDBusClient = KuhlerProfilDBusClient;

/**
 * KühlerProfil Quick Menu Toggle for GNOME 45+ Quick Settings panel.
 */
export const KuhlerProfilToggle = GObject.registerClass(
class KuhlerProfilToggle extends QuickSettings.QuickMenuToggle {
    _init(extension, client, statusIndicator) {
        super._init({
            title: safeTranslate('KühlerProfil'),
            subtitle: safeTranslate('Connecting…'),
            iconName: 'power-profile-balanced-symbolic',
            toggleMode: true,
            menuButtonAccessibleName: safeTranslate('Open KühlerProfil menu'),
        });

        this._extension = extension;
        this._client = client;
        this._statusIndicator = statusIndicator;
        this._modeButtons = new Map();
        this._batteryButtons = new Map();
        this._curveButtons = new Map();
        this._cooldownButtons = new Map();
        this._syncingAutoSwitch = false;
        this._unsubscribe = null;

        this._buildMenu();

        // Connect main toggle click to cycle mode or toggle auto
        this.connect('clicked', () => {
            this._onMainToggleClicked();
        });

        this._unsubscribe = this._client.subscribe((telemetry, connected) => {
            this._updateUI(telemetry, connected);
        });
    }

    _buildMenu() {
        // Set Header
        this.menu.setHeader(
            'power-profile-balanced-symbolic',
            safeTranslate('KühlerProfil'),
            safeTranslate('ASUS Thermal & Battery Care')
        );

        // Header settings button for instant Libadwaita Preferences access (GNOME HIG)
        const headerSettingsBtn = new St.Button({
            style_class: 'icon-button',
            child: new St.Icon({
                icon_name: 'emblem-system-symbolic',
                style_class: 'popup-menu-icon',
            }),
            can_focus: true,
            accessible_name: safeTranslate('Open Preferences'),
        });
        headerSettingsBtn.connect('clicked', () => {
            try {
                Main.panel.closeQuickSettings();
                this._extension.openPreferences();
            } catch (err) {
                console.error(`[KühlerProfil] Failed to open preferences: ${err}`);
            }
        });
        this.menu.addHeaderSuffix(headerSettingsBtn);

        // Telemetry Section
        this._buildTelemetrySection();

        // Separator
        this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        // Thermal Profile Section
        this._buildThermalModeSection();

        // Separator
        this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        // Dynamic Auto Governor & Fan Curve Section
        this._buildCurveProfileSection();

        // Separator
        this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        // Zero-RPM Cooldown Mode Section
        this._buildCooldownSection();

        // Separator
        this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        // Battery Care Limit Section
        this._buildBatteryLimitSection();

        // Separator
        this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        // Status & Connection Footer with Preferences Action
        this._buildStatusFooter();
    }

    _buildTelemetrySection() {
        const item = new PopupMenu.PopupBaseMenuItem({
            reactive: false,
            can_focus: false,
        });

        const gridBox = new St.BoxLayout({
            vertical: true,
            style_class: 'kuhlerprofil-telemetry-grid',
            x_expand: true,
        });

        // Row 1: CPU Temp & Battery
        const row1 = new St.BoxLayout({
            vertical: false,
            style_class: 'kuhlerprofil-telemetry-row',
            x_expand: true,
        });

        this._tempCard = new St.BoxLayout({
            vertical: true,
            style_class: 'kuhlerprofil-metric-card',
            x_expand: true,
        });
        const tempLabel = new St.Label({
            text: safeTranslate('CPU Temperature'),
            style_class: 'kuhlerprofil-metric-label',
        });
        this._tempValue = new St.Label({
            text: '-- °C',
            style_class: 'kuhlerprofil-metric-value',
        });
        this._tempCard.add_child(tempLabel);
        this._tempCard.add_child(this._tempValue);

        this._batCard = new St.BoxLayout({
            vertical: true,
            style_class: 'kuhlerprofil-metric-card',
            x_expand: true,
        });
        const batLabel = new St.Label({
            text: safeTranslate('Battery & Power'),
            style_class: 'kuhlerprofil-metric-label',
        });
        this._batValue = new St.Label({
            text: '-- % (AC)',
            style_class: 'kuhlerprofil-metric-value',
        });
        this._batCard.add_child(batLabel);
        this._batCard.add_child(this._batValue);

        row1.add_child(this._tempCard);
        row1.add_child(this._batCard);

        // Row 2: Fan 1 & Fan 2 RPM
        const row2 = new St.BoxLayout({
            vertical: false,
            style_class: 'kuhlerprofil-telemetry-row',
            x_expand: true,
        });

        this._fan1Card = new St.BoxLayout({
            vertical: true,
            style_class: 'kuhlerprofil-metric-card',
            x_expand: true,
        });
        const fan1Label = new St.Label({
            text: safeTranslate('CPU Fan (Fan 1)'),
            style_class: 'kuhlerprofil-metric-label',
        });
        this._fan1Value = new St.Label({
            text: '-- RPM',
            style_class: 'kuhlerprofil-metric-value',
        });
        this._fan1Card.add_child(fan1Label);
        this._fan1Card.add_child(this._fan1Value);

        this._fan2Card = new St.BoxLayout({
            vertical: true,
            style_class: 'kuhlerprofil-metric-card',
            x_expand: true,
        });
        const fan2Label = new St.Label({
            text: safeTranslate('GPU Fan (Fan 2)'),
            style_class: 'kuhlerprofil-metric-label',
        });
        this._fan2Value = new St.Label({
            text: '-- RPM',
            style_class: 'kuhlerprofil-metric-value',
        });
        this._fan2Card.add_child(fan2Label);
        this._fan2Card.add_child(this._fan2Value);

        row2.add_child(this._fan1Card);
        row2.add_child(this._fan2Card);

        gridBox.add_child(row1);
        gridBox.add_child(row2);
        item.add_child(gridBox);
        this.menu.addMenuItem(item);
    }

    _buildThermalModeSection() {
        const item = new PopupMenu.PopupBaseMenuItem({
            reactive: false,
            can_focus: false,
        });

        const container = new St.BoxLayout({
            vertical: true,
            style_class: 'kuhlerprofil-section',
            x_expand: true,
        });

        const title = new St.Label({
            text: safeTranslate('ASUS Thermal Profile'),
            style_class: 'kuhlerprofil-section-title',
        });
        container.add_child(title);

        const btnGroup = new St.BoxLayout({
            vertical: false,
            style_class: 'kuhlerprofil-button-group',
            x_expand: true,
        });

        for (const mode of THERMAL_MODES) {
            const btn = new St.Button({
                label: mode.name,
                style_class: 'kuhlerprofil-mode-button',
                can_focus: true,
                toggle_mode: true,
                accessible_role: Atk.Role.RADIO_BUTTON,
                accessible_name: `${mode.name} mode`,
                x_expand: true,
            });

            btn.connect('clicked', () => {
                this._client.setThermalMode(mode.id).catch(err => {
                    console.error(`[KühlerProfil] Failed to set mode ${mode.id}: ${err}`);
                });
            });

            this._modeButtons.set(mode.id, btn);
            btnGroup.add_child(btn);
        }

        container.add_child(btnGroup);
        item.add_child(container);
        this.menu.addMenuItem(item);
    }

    _buildCurveProfileSection() {
        // Auto Governor Switch
        this._autoSwitch = new PopupMenu.PopupSwitchMenuItem(
            safeTranslate('Dynamic Auto-Governor'),
            false
        );

        this._autoSwitch.connect('toggled', (item, state) => {
            if (this._syncingAutoSwitch) return;
            this._client.setAutoMode(state).catch(err => {
                console.error(`[KühlerProfil] Failed to set auto mode: ${err}`);
                this._syncingAutoSwitch = true;
                try {
                    item.setToggleState(!state);
                } finally {
                    this._syncingAutoSwitch = false;
                }
            });
        });

        this.menu.addMenuItem(this._autoSwitch);

        // Curve Profile Selectors
        const item = new PopupMenu.PopupBaseMenuItem({
            reactive: false,
            can_focus: false,
        });

        const container = new St.BoxLayout({
            vertical: true,
            style_class: 'kuhlerprofil-section',
            x_expand: true,
        });

        const headerBox = new St.BoxLayout({
            vertical: false,
            x_expand: true,
        });

        const title = new St.Label({
            text: safeTranslate('Fan Curve Profile'),
            style_class: 'kuhlerprofil-section-title',
            x_expand: true,
        });
        headerBox.add_child(title);

        this._hwCurveBadge = new St.Label({
            text: safeTranslate('Software Governor'),
            style_class: 'kuhlerprofil-badge',
        });
        headerBox.add_child(this._hwCurveBadge);
        container.add_child(headerBox);

        const btnGroup = new St.BoxLayout({
            vertical: false,
            style_class: 'kuhlerprofil-button-group',
            x_expand: true,
        });

        for (const cp of CURVE_PROFILES) {
            const btn = new St.Button({
                label: cp.name,
                style_class: 'kuhlerprofil-mode-button',
                can_focus: true,
                toggle_mode: true,
                accessible_role: Atk.Role.RADIO_BUTTON,
                accessible_name: `${cp.name} curve profile`,
                x_expand: true,
            });

            btn.connect('clicked', () => {
                this._client.setCurveProfile(cp.id).catch(err => {
                    console.error(`[KühlerProfil] Failed to set curve profile ${cp.id}: ${err}`);
                });
            });

            this._curveButtons.set(cp.id, btn);
            btnGroup.add_child(btn);
        }

        container.add_child(btnGroup);
        item.add_child(container);
        this.menu.addMenuItem(item);
    }

    _buildCooldownSection() {
        const item = new PopupMenu.PopupBaseMenuItem({
            reactive: false,
            can_focus: false,
        });

        const container = new St.BoxLayout({
            vertical: true,
            style_class: 'kuhlerprofil-section',
            x_expand: true,
        });

        const headerBox = new St.BoxLayout({
            vertical: false,
            x_expand: true,
        });

        const title = new St.Label({
            text: safeTranslate('Zero-RPM Cooldown'),
            style_class: 'kuhlerprofil-section-title',
            x_expand: true,
        });
        headerBox.add_child(title);
        container.add_child(headerBox);

        const btnGroup = new St.BoxLayout({
            vertical: false,
            style_class: 'kuhlerprofil-button-group',
            x_expand: true,
        });

        for (const cm of COOLDOWN_MODES) {
            const btn = new St.Button({
                label: cm.name,
                style_class: 'kuhlerprofil-mode-button',
                can_focus: true,
                toggle_mode: true,
                accessible_role: Atk.Role.RADIO_BUTTON,
                accessible_name: `${cm.name} cooldown mode`,
                x_expand: true,
            });

            btn.connect('clicked', () => {
                this._client.setCooldownMode(cm.id).catch(err => {
                    console.error(`[KühlerProfil] Failed to set cooldown mode ${cm.id}: ${err}`);
                });
            });

            this._cooldownButtons.set(cm.id, btn);
            btnGroup.add_child(btn);
        }

        container.add_child(btnGroup);
        item.add_child(container);
        this.menu.addMenuItem(item);
    }

    _buildBatteryLimitSection() {
        const item = new PopupMenu.PopupBaseMenuItem({
            reactive: false,
            can_focus: false,
        });

        const container = new St.BoxLayout({
            vertical: true,
            style_class: 'kuhlerprofil-section',
            x_expand: true,
        });

        const title = new St.Label({
            text: safeTranslate('Battery Health Charge Limit'),
            style_class: 'kuhlerprofil-section-title',
        });
        container.add_child(title);

        const btnGroup = new St.BoxLayout({
            vertical: false,
            style_class: 'kuhlerprofil-button-group',
            x_expand: true,
        });

        for (const limit of BATTERY_LIMITS) {
            const btn = new St.Button({
                label: `${limit}%`,
                style_class: 'kuhlerprofil-battery-button',
                can_focus: true,
                toggle_mode: true,
                accessible_role: Atk.Role.RADIO_BUTTON,
                accessible_name: `Battery limit ${limit}%`,
                x_expand: true,
            });

            btn.connect('clicked', () => {
                this._client.setBatteryLimit(limit).catch(err => {
                    console.error(`[KühlerProfil] Failed to set battery limit ${limit}%: ${err}`);
                });
            });

            this._batteryButtons.set(limit, btn);
            btnGroup.add_child(btn);
        }

        container.add_child(btnGroup);
        item.add_child(container);
        this.menu.addMenuItem(item);
    }

    _buildStatusFooter() {
        // Daemon connection status row
        const statusItem = new PopupMenu.PopupBaseMenuItem({
            reactive: true,
            can_focus: true,
        });

        const box = new St.BoxLayout({
            vertical: false,
            style_class: 'kuhlerprofil-status-bar',
            x_expand: true,
        });

        this._statusLabel = new St.Label({
            text: safeTranslate('● kuhlerprofild connected'),
            style_class: 'kuhlerprofil-status-text kuhlerprofil-status-ok',
            x_expand: true,
        });

        box.add_child(this._statusLabel);
        statusItem.add_child(box);

        statusItem.connect('activate', () => {
            this._client.getStatus().catch(() => {});
        });

        this.menu.addMenuItem(statusItem);

        // Preferences action button
        this.menu.addAction(safeTranslate('Preferences…'), () => {
            try {
                Main.panel.closeQuickSettings();
                this._extension.openPreferences();
            } catch (err) {
                console.error(`[KühlerProfil] Failed to open preferences: ${err}`);
            }
        });
    }

    _onMainToggleClicked() {
        if (!this._client._lastTelemetry) return;
        const currentMode = this._client._lastTelemetry.activeMode;
        // Cycle mode: silent -> standard -> boost -> silent
        let nextMode = 'standard';
        if (currentMode === 'silent') nextMode = 'standard';
        else if (currentMode === 'standard') nextMode = 'boost';
        else if (currentMode === 'boost') nextMode = 'silent';

        this._client.setThermalMode(nextMode).catch(err => {
            console.error(`[KühlerProfil] Failed to cycle mode: ${err}`);
        });
    }

    _updateUI(telemetry, connected) {
        if (!connected || !telemetry) {
            this.subtitle = safeTranslate('Daemon Disconnected');
            this.checked = false;
            this.iconName = 'power-profile-balanced-symbolic';
            if (this._statusIndicator) {
                this._statusIndicator.iconName = 'power-profile-balanced-symbolic';
            }

            if (this._statusLabel) {
                this._statusLabel.text = safeTranslate('⚠ Daemon offline (Click to retry)');
                this._statusLabel.style_class = 'kuhlerprofil-status-text kuhlerprofil-status-warn';
            }
            return;
        }

        const modeInfo = getModeInfo(telemetry.activeMode);
        this.iconName = modeInfo.icon;
        if (this._statusIndicator) {
            this._statusIndicator.iconName = modeInfo.icon;
        }

        // Subtitle: "Standard · 52°C · 2400 RPM" (or with [Curve] in Auto mode)
        const tempText = telemetry.cpuTemp > 0 ? `${Math.round(telemetry.cpuTemp)}°C` : '--°C';
        const rpmText = telemetry.fan1Rpm > 0 ? `${telemetry.fan1Rpm} RPM` : '0 RPM';
        const curveName = telemetry.activeCurve ? (telemetry.activeCurve.charAt(0).toUpperCase() + telemetry.activeCurve.slice(1)) : 'Balanced';
        this.subtitle = telemetry.autoMode
            ? `${modeInfo.name} · ${tempText} · ${rpmText} [${curveName}]`
            : `${modeInfo.name} · ${tempText} · ${rpmText}`;
        this.checked = telemetry.autoMode || telemetry.activeMode === 'boost';

        // Update Telemetry Card values with dynamic temperature styling
        if (this._tempValue) {
            this._tempValue.text = telemetry.cpuTemp > 0 ? `${telemetry.cpuTemp.toFixed(1)} °C` : '-- °C';
            this._tempValue.remove_style_class_name('kuhlerprofil-temp-cool');
            this._tempValue.remove_style_class_name('kuhlerprofil-temp-normal');
            this._tempValue.remove_style_class_name('kuhlerprofil-temp-warm');
            this._tempValue.remove_style_class_name('kuhlerprofil-temp-hot');

            if (telemetry.cpuTemp > 0) {
                if (telemetry.cpuTemp < 50) {
                    this._tempValue.add_style_class_name('kuhlerprofil-temp-cool');
                } else if (telemetry.cpuTemp < 65) {
                    this._tempValue.add_style_class_name('kuhlerprofil-temp-normal');
                } else if (telemetry.cpuTemp < 75) {
                    this._tempValue.add_style_class_name('kuhlerprofil-temp-warm');
                } else {
                    this._tempValue.add_style_class_name('kuhlerprofil-temp-hot');
                }
            }
        }

        if (this._batValue) {
            const acStatus = telemetry.onAC ? safeTranslate('AC Plugged') : safeTranslate('Battery');
            this._batValue.text = `${telemetry.batteryPercent}% (${acStatus})`;
        }
        if (this._fan1Value) {
            this._fan1Value.text = telemetry.fan1Rpm > 0 ? `${telemetry.fan1Rpm.toLocaleString()} RPM` : '0 RPM';
        }
        if (this._fan2Value) {
            this._fan2Value.text = telemetry.fan2Rpm > 0 ? `${telemetry.fan2Rpm.toLocaleString()} RPM` : '0 RPM';
        }

        // Update Thermal Mode Button active styles
        for (const [modeId, btn] of this._modeButtons.entries()) {
            const isSelected = (modeId === telemetry.activeMode);
            btn.checked = isSelected;
            if (isSelected) {
                btn.add_style_class_name('kuhlerprofil-button-active');
                btn.add_style_class_name('koolthing-button-active');
            } else {
                btn.remove_style_class_name('kuhlerprofil-button-active');
                btn.remove_style_class_name('koolthing-button-active');
            }
        }

        // Update Battery Limit Button active styles
        for (const [limitVal, btn] of this._batteryButtons.entries()) {
            const isSelected = (limitVal === telemetry.batteryLimit);
            btn.checked = isSelected;
            if (isSelected) {
                btn.add_style_class_name('kuhlerprofil-button-active');
                btn.add_style_class_name('koolthing-button-active');
            } else {
                btn.remove_style_class_name('kuhlerprofil-button-active');
                btn.remove_style_class_name('koolthing-button-active');
            }
        }

        // Update Curve Profile Button active styles
        for (const [curveId, btn] of this._curveButtons.entries()) {
            const isSelected = (curveId === telemetry.activeCurve);
            btn.checked = isSelected;
            if (isSelected) {
                btn.add_style_class_name('kuhlerprofil-button-active');
                btn.add_style_class_name('koolthing-button-active');
            } else {
                btn.remove_style_class_name('kuhlerprofil-button-active');
                btn.remove_style_class_name('koolthing-button-active');
            }
        }

        // Update Cooldown Mode Button active styles
        for (const [cooldownId, btn] of this._cooldownButtons.entries()) {
            const isSelected = (cooldownId === telemetry.cooldownMode);
            btn.checked = isSelected;
            if (isSelected) {
                btn.add_style_class_name('kuhlerprofil-button-active');
                btn.add_style_class_name('koolthing-button-active');
            } else {
                btn.remove_style_class_name('kuhlerprofil-button-active');
                btn.remove_style_class_name('koolthing-button-active');
            }
        }

        // Update Hardware Curve Badge
        if (this._hwCurveBadge) {
            this._hwCurveBadge.text = telemetry.hasHardwareCurve
                ? safeTranslate('ASUS ACPI HW')
                : safeTranslate('Software Governor');
        }

        // Update Auto Governor Switch (guarded against feedback loop)
        if (this._autoSwitch && this._autoSwitch.state !== telemetry.autoMode) {
            this._syncingAutoSwitch = true;
            try {
                this._autoSwitch.setToggleState(telemetry.autoMode);
            } finally {
                this._syncingAutoSwitch = false;
            }
        }

        // Update Footer Status
        if (this._statusLabel) {
            const govStatus = telemetry.autoMode
                ? safeTranslate('Auto Governor Active')
                : safeTranslate('Manual Profile');
            this._statusLabel.text = `● kuhlerprofild connected (${govStatus})`;
            this._statusLabel.style_class = 'kuhlerprofil-status-text kuhlerprofil-status-ok';
        }
    }

    destroy() {
        if (this._unsubscribe) {
            this._unsubscribe();
            this._unsubscribe = null;
        }
        this._modeButtons.clear();
        this._batteryButtons.clear();
        this._curveButtons.clear();
        this._cooldownButtons.clear();
        if (this.menu) {
            this.menu.destroy();
        }
        super.destroy();
    }
});

export const KoolThingToggle = KuhlerProfilToggle;

/**
 * KühlerProfil System Indicator for GNOME 45+ Quick Settings.
 */
export const KuhlerProfilIndicator = GObject.registerClass(
class KuhlerProfilIndicator extends QuickSettings.SystemIndicator {
    _init(extension, client) {
        super._init();
        this._extension = extension;
        this._client = client;

        this._indicator = this._addIndicator();
        this._indicator.iconName = 'power-profile-balanced-symbolic';
        this._indicator.visible = true;

        this._toggle = new KuhlerProfilToggle(extension, client, this._indicator);
        this._toggle.bind_property('icon-name',
            this._indicator, 'icon-name',
            GObject.BindingFlags.SYNC_CREATE);
        this.quickSettingsItems.push(this._toggle);
    }

    destroy() {
        if (this._toggle) {
            this._toggle.destroy();
            this._toggle = null;
        }
        this.quickSettingsItems = [];
        super.destroy();
    }
});

export const KoolThingIndicator = KuhlerProfilIndicator;

/**
 * KühlerProfil Extension entry point.
 */
export default class KuhlerProfilExtension extends Extension {
    enable() {
        this.initTranslations();
        this._client = new KuhlerProfilDBusClient();
        this._indicator = new KuhlerProfilIndicator(this, this._client);

        Main.panel.statusArea.quickSettings.addExternalIndicator(this._indicator);
        this._client.start();
    }

    disable() {
        if (this._client) {
            this._client.stop();
            this._client = null;
        }
        if (this._indicator) {
            this._indicator.destroy();
            this._indicator = null;
        }
    }
}

export { KuhlerProfilExtension as KoolThingExtension };
