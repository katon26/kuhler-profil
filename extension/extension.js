/**
 * KühlerProfil GNOME Shell Quick Settings Extension
 * Provides real-time thermal monitoring, fan RPM telemetry, ASUS thermal profile
 * switching, and battery charge threshold control over Linux D-Bus IPC.
 *
 * Supported GNOME Shell Versions: 45, 46, 47, 48, 49, 50
 */

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
    <signal name="ThermalModeChanged">
      <arg name="mode" type="s"/>
    </signal>
    <signal name="BatteryLimitChanged">
      <arg name="limit" type="i"/>
    </signal>
    <signal name="TelemetryTick">
      <arg name="status" type="a{sv}"/>
    </signal>
  </interface>
</node>`;

export const KoolThingInterfaceXML = KuhlerProfilInterfaceXML;

export const THERMAL_MODES = [
    { id: 'silent', name: _('Silent'), icon: 'power-profile-power-saver-symbolic', desc: _('Quiet') },
    { id: 'standard', name: _('Standard'), icon: 'power-profile-balanced-symbolic', desc: _('Balanced') },
    { id: 'boost', name: _('Boost'), icon: 'power-profile-performance-symbolic', desc: _('Max Cooling') },
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
    };
}

/**
 * Formats a thermal mode ID into display text and matching symbolic icon.
 */
export function getModeInfo(mode) {
    const normalized = String(mode || 'standard').toLowerCase();
    const found = THERMAL_MODES.find(m => m.id === normalized);
    if (found) return found;
    return { id: normalized, name: normalized.charAt(0).toUpperCase() + normalized.slice(1), icon: 'power-profile-balanced-symbolic', desc: '' };
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
        this._reconnectTimerId = 0;
        this._connection = null;
        this._connected = false;
        this._lastTelemetry = null;
    }

    start() {
        this._initProxy();
        // Poll every 3 seconds as fallback / watchdog for live metrics
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
        if (this._reconnectTimerId) {
            GLib.source_remove(this._reconnectTimerId);
            this._reconnectTimerId = 0;
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
                    this._proxy.disconnect(id);
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
            
            // Try System bus first (standard for kuhlerprofild daemon)
            let bus = Gio.DBus.system;
            if (!bus) {
                bus = Gio.DBus.session;
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
            // Connect to signal proxies
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
}

export const KoolThingDBusClient = KuhlerProfilDBusClient;

/**
 * KühlerProfil Quick Menu Toggle for GNOME 45+ Quick Settings panel.
 */
export const KuhlerProfilToggle = GObject.registerClass(
class KuhlerProfilToggle extends QuickSettings.QuickMenuToggle {
    _init(extension, client, statusIndicator) {
        super._init({
            title: _('KühlerProfil'),
            subtitle: _('Connecting...'),
            iconName: 'power-profile-balanced-symbolic',
            toggleMode: true,
        });

        this._extension = extension;
        this._client = client;
        this._statusIndicator = statusIndicator;
        this._modeButtons = new Map();
        this._batteryButtons = new Map();
        this._unsubscribe = null;

        this._buildMenu();

        // Connect main toggle click to switch mode / toggle auto
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
            _('KühlerProfil'),
            _('Thermal Governor & Battery Health')
        );

        // Telemetry Section
        this._buildTelemetrySection();

        // Separator
        this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        // Thermal Profile Section
        this._buildThermalModeSection();

        // Separator
        this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        // Battery Care Limit Section
        this._buildBatteryLimitSection();

        // Separator
        this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        // Auto Governor Switch
        this._buildAutoGovernorSection();

        // Separator
        this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());

        // Status & Connection Footer
        this._buildStatusFooter();
    }

    _buildTelemetrySection() {
        const item = new PopupMenu.PopupBaseMenuItem({
            reactive: false,
            can_focus: false,
        });

        const gridBox = new St.BoxLayout({
            vertical: true,
            style_class: 'koolthing-telemetry-grid',
            x_expand: true,
        });

        // Row 1: CPU Temp & Battery
        const row1 = new St.BoxLayout({
            vertical: false,
            style_class: 'koolthing-telemetry-row',
            x_expand: true,
        });

        this._tempCard = new St.BoxLayout({
            vertical: true,
            style_class: 'koolthing-metric-card',
            x_expand: true,
        });
        const tempLabel = new St.Label({ text: _('CPU Temperature'), style_class: 'koolthing-metric-label' });
        this._tempValue = new St.Label({ text: '-- °C', style_class: 'koolthing-metric-value' });
        this._tempCard.add_child(tempLabel);
        this._tempCard.add_child(this._tempValue);

        this._batCard = new St.BoxLayout({
            vertical: true,
            style_class: 'koolthing-metric-card',
            x_expand: true,
        });
        const batLabel = new St.Label({ text: _('Battery Level'), style_class: 'koolthing-metric-label' });
        this._batValue = new St.Label({ text: '-- % (AC)', style_class: 'koolthing-metric-value' });
        this._batCard.add_child(batLabel);
        this._batCard.add_child(this._batValue);

        row1.add_child(this._tempCard);
        row1.add_child(this._batCard);

        // Row 2: Fan 1 & Fan 2 RPM
        const row2 = new St.BoxLayout({
            vertical: false,
            style_class: 'koolthing-telemetry-row',
            x_expand: true,
        });

        this._fan1Card = new St.BoxLayout({
            vertical: true,
            style_class: 'koolthing-metric-card',
            x_expand: true,
        });
        const fan1Label = new St.Label({ text: _('CPU Fan (Fan 1)'), style_class: 'koolthing-metric-label' });
        this._fan1Value = new St.Label({ text: '-- RPM', style_class: 'koolthing-metric-value' });
        this._fan1Card.add_child(fan1Label);
        this._fan1Card.add_child(this._fan1Value);

        this._fan2Card = new St.BoxLayout({
            vertical: true,
            style_class: 'koolthing-metric-card',
            x_expand: true,
        });
        const fan2Label = new St.Label({ text: _('GPU Fan (Fan 2)'), style_class: 'koolthing-metric-label' });
        this._fan2Value = new St.Label({ text: '-- RPM', style_class: 'koolthing-metric-value' });
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
            style_class: 'koolthing-section',
            x_expand: true,
        });

        const title = new St.Label({
            text: _('Thermal Profile'),
            style_class: 'koolthing-section-title',
        });
        container.add_child(title);

        const btnGroup = new St.BoxLayout({
            vertical: false,
            style_class: 'koolthing-button-group',
            x_expand: true,
        });

        for (const mode of THERMAL_MODES) {
            const btn = new St.Button({
                label: mode.name,
                style_class: 'koolthing-mode-button',
                can_focus: true,
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

    _buildBatteryLimitSection() {
        const item = new PopupMenu.PopupBaseMenuItem({
            reactive: false,
            can_focus: false,
        });

        const container = new St.BoxLayout({
            vertical: true,
            style_class: 'koolthing-section',
            x_expand: true,
        });

        const title = new St.Label({
            text: _('Battery Health Limit'),
            style_class: 'koolthing-section-title',
        });
        container.add_child(title);

        const btnGroup = new St.BoxLayout({
            vertical: false,
            style_class: 'koolthing-button-group',
            x_expand: true,
        });

        for (const limit of BATTERY_LIMITS) {
            const btn = new St.Button({
                label: `${limit}%`,
                style_class: 'koolthing-battery-button',
                can_focus: true,
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

    _buildAutoGovernorSection() {
        this._autoSwitch = new PopupMenu.PopupSwitchMenuItem(
            _('Dynamic Auto Governor'),
            false
        );

        this._autoSwitch.connect('toggled', (item, state) => {
            this._client.setAutoMode(state).catch(err => {
                console.error(`[KühlerProfil] Failed to set auto mode: ${err}`);
                item.setToggleState(!state);
            });
        });

        this.menu.addMenuItem(this._autoSwitch);
    }

    _buildStatusFooter() {
        const item = new PopupMenu.PopupBaseMenuItem({
            reactive: true,
            can_focus: true,
        });

        const box = new St.BoxLayout({
            vertical: false,
            style_class: 'koolthing-status-bar',
            x_expand: true,
        });

        this._statusLabel = new St.Label({
            text: _('● kuhlerprofild connected'),
            style_class: 'koolthing-status-text koolthing-status-ok',
            x_expand: true,
        });

        box.add_child(this._statusLabel);
        item.add_child(box);

        item.connect('activate', () => {
            this._client.getStatus().catch(() => {});
        });

        this.menu.addMenuItem(item);
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
            this.subtitle = _('Daemon Disconnected');
            this.checked = false;
            this.iconName = 'power-profile-balanced-symbolic';
            if (this._statusIndicator) {
                this._statusIndicator.iconName = 'power-profile-balanced-symbolic';
            }

            if (this._statusLabel) {
                this._statusLabel.text = _('⚠ Daemon offline (Click to retry)');
                this._statusLabel.style_class = 'koolthing-status-text koolthing-status-warn';
            }
            return;
        }

        const modeInfo = getModeInfo(telemetry.activeMode);
        this.iconName = modeInfo.icon;
        if (this._statusIndicator) {
            this._statusIndicator.iconName = modeInfo.icon;
        }

        // Subtitle: "Standard · 52°C · 2400 RPM"
        const tempText = telemetry.cpuTemp > 0 ? `${Math.round(telemetry.cpuTemp)}°C` : '--°C';
        const rpmText = telemetry.fan1Rpm > 0 ? `${telemetry.fan1Rpm} RPM` : '0 RPM';
        this.subtitle = `${modeInfo.name} · ${tempText} · ${rpmText}`;
        this.checked = telemetry.autoMode || telemetry.activeMode === 'boost';

        // Update Telemetry Card values
        if (this._tempValue) {
            this._tempValue.text = telemetry.cpuTemp > 0 ? `${telemetry.cpuTemp.toFixed(1)} °C` : '-- °C';
        }
        if (this._batValue) {
            const acStatus = telemetry.onAC ? _('AC Plugged') : _('Battery');
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
            if (modeId === telemetry.activeMode) {
                btn.add_style_class_name('koolthing-button-active');
            } else {
                btn.remove_style_class_name('koolthing-button-active');
            }
        }

        // Update Battery Limit Button active styles
        for (const [limitVal, btn] of this._batteryButtons.entries()) {
            if (limitVal === telemetry.batteryLimit) {
                btn.add_style_class_name('koolthing-button-active');
            } else {
                btn.remove_style_class_name('koolthing-button-active');
            }
        }

        // Update Auto Governor Switch
        if (this._autoSwitch && this._autoSwitch.state !== telemetry.autoMode) {
            this._autoSwitch.setToggleState(telemetry.autoMode);
        }

        // Update Footer Status
        if (this._statusLabel) {
            const govStatus = telemetry.autoMode ? _('Auto Governor Active') : _('Manual Profile');
            this._statusLabel.text = `● kuhlerprofild connected (${govStatus})`;
            this._statusLabel.style_class = 'koolthing-status-text koolthing-status-ok';
        }
    }

    destroy() {
        if (this._unsubscribe) {
            this._unsubscribe();
            this._unsubscribe = null;
        }
        this._modeButtons.clear();
        this._batteryButtons.clear();
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
        this.quickSettingsItems.push(this._toggle);
    }

    destroy() {
        if (this._toggle) {
            this._toggle.destroy();
            this._toggle = null;
        }
        super.destroy();
    }
});

export const KoolThingIndicator = KuhlerProfilIndicator;

/**
 * KühlerProfil Extension entry point.
 */
export default class KuhlerProfilExtension extends Extension {
    enable() {
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
            this._indicator.quickSettingsItems.forEach(item => item.destroy());
            this._indicator.destroy();
            this._indicator = null;
        }
    }
}

export { KuhlerProfilExtension as KoolThingExtension };
