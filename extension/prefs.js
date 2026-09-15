/**
 * KühlerProfil Extension Preferences (GNOME 45+)
 * Built with Libadwaita & GTK4 for native GNOME Settings integration.
 */

import Adw from 'gi://Adw';
import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import GObject from 'gi://GObject';
import Gtk from 'gi://Gtk?version=4.0';
import Gdk from 'gi://Gdk?version=4.0';

import { ExtensionPreferences, gettext as _ } from 'resource:///org/gnome/Shell/Extensions/js/extensions/prefs.js';

const DBUS_NAME = 'org.freedesktop.kuhlerprofil';
const DBUS_PATH = '/org/freedesktop/kuhlerprofil';

const KuhlerProfilIfaceXML = `
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
    <method name="GetActiveCurveProfile">
      <arg name="profile" type="s" direction="out"/>
    </method>
    <method name="SetCurveProfile">
      <arg name="profile" type="s" direction="in"/>
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

function safeTranslate(str) {
    try {
        if (typeof _ === 'function') {
            return _(str);
        }
    } catch (_) {}
    return str;
}

function getSafeBus() {
    try {
        if (Gio.DBus.system) return Gio.DBus.system;
    } catch (_) {}
    try {
        if (Gio.DBus.session) return Gio.DBus.session;
    } catch (_) {}
    return null;
}

function unwrapVariant(variant) {
    if (variant === null || variant === undefined) return variant;
    if (typeof variant.deepUnpack === 'function') return variant.deepUnpack();
    if (typeof variant.unpack === 'function') return unwrapVariant(variant.unpack());
    if (Array.isArray(variant)) return variant.map(unwrapVariant);
    if (typeof variant === 'object') {
        const res = {};
        for (const [k, v] of Object.entries(variant)) {
            res[k] = unwrapVariant(v);
        }
        return res;
    }
    return variant;
}

function launchUri(window, url) {
    try {
        Gtk.show_uri(window, url, Gdk.CURRENT_TIME);
    } catch (error) {
        console.error(`[KühlerProfil] Failed to open URI ${url}: ${error}`);
    }
}

function createLinkRow(window, title, url) {
    const row = new Adw.ActionRow({
        title,
        activatable: true,
    });
    row.add_suffix(new Gtk.Image({ icon_name: 'external-link-symbolic' }));
    row.connect('activated', () => {
        launchUri(window, url);
    });
    return row;
}

function openLegalDialog(window, baseUrl, gettextFunc) {
    const dialog = new Adw.Dialog({
        content_width: 420,
        content_height: 560,
        presentation_mode: Adw.DialogPresentationMode.BOTTOM_SHEET,
    });

    const toolbarView = new Adw.ToolbarView();
    const headerBar = new Adw.HeaderBar({
        show_title: true,
        title_widget: new Adw.WindowTitle({ title: gettextFunc('Legal') }),
    });
    toolbarView.add_top_bar(headerBar);

    const legalPage = new Adw.PreferencesPage();

    const licenseGroup = new Adw.PreferencesGroup({
        title: gettextFunc('License'),
        description: gettextFunc('KühlerProfil is free and open source software.'),
    });
    licenseGroup.add(
        createLinkRow(
            window,
            gettextFunc('MIT License'),
            `${baseUrl}/blob/main/LICENSE`
        )
    );
    legalPage.add(licenseGroup);

    const copyrightGroup = new Adw.PreferencesGroup({
        title: gettextFunc('Copyright'),
        description: gettextFunc('Copyright © 2026 katon26. Licensed under the terms of the MIT License.'),
    });
    legalPage.add(copyrightGroup);

    const scroller = new Gtk.ScrolledWindow({ vexpand: true, hexpand: true });
    scroller.set_child(legalPage);
    toolbarView.set_content(scroller);
    dialog.set_child(toolbarView);

    dialog.present(window);
}

function createLegalRow(window, baseUrl, gettextFunc) {
    const row = new Adw.ActionRow({
        title: gettextFunc('Legal'),
        activatable: true,
    });
    row.add_suffix(new Gtk.Image({ icon_name: 'go-next-symbolic' }));
    row.connect('activated', () => {
        openLegalDialog(window, baseUrl, gettextFunc);
    });
    return row;
}

function createAboutPage(window, extensionPath, metadata, gettextFunc) {
    const aboutPage = new Adw.PreferencesPage({
        title: gettextFunc('About'),
        icon_name: 'help-about-symbolic',
        name: 'AboutPage',
    });

    const headerGroup = new Adw.PreferencesGroup();
    const headerBox = new Gtk.Box({
        orientation: Gtk.Orientation.VERTICAL,
        spacing: 12,
        margin_top: 16,
        margin_bottom: 8,
        margin_start: 16,
        margin_end: 16,
        halign: Gtk.Align.CENTER,
    });

    // App Logo
    const logoCandidates = [
        GLib.build_filenamev([extensionPath ?? '.', 'src', 'kuhlerprofil-logo.svg']),
    ];
    let logoLoaded = false;
    for (const candPath of logoCandidates) {
        const candFile = Gio.File.new_for_path(candPath);
        if (candFile.query_exists(null)) {
            const logoImage = new Gtk.Picture({
                file: candFile,
                width_request: 128,
                height_request: 128,
                content_fit: Gtk.ContentFit.CONTAIN,
                halign: Gtk.Align.CENTER,
            });
            headerBox.append(logoImage);
            logoLoaded = true;
            break;
        }
    }
    if (!logoLoaded) {
        const fallbackIcon = new Gtk.Image({
            icon_name: 'power-profile-balanced-symbolic',
            pixel_size: 96,
            halign: Gtk.Align.CENTER,
        });
        headerBox.append(fallbackIcon);
    }

    const extensionName = metadata?.name ?? 'KühlerProfil';
    headerBox.append(
        new Gtk.Label({
            label: `<span size="xx-large" weight="bold">${GLib.markup_escape_text(extensionName, -1)}</span>`,
            use_markup: true,
            halign: Gtk.Align.CENTER,
        })
    );

    headerBox.append(
        new Gtk.Label({
            label: '<a href="https://katon26.github.io">Katon (katon26)</a>',
            use_markup: true,
            halign: Gtk.Align.CENTER,
        })
    );

    const rawVersionName = metadata?.['version-name'] ?? null;
    const versionNumber =
        metadata?.version !== undefined && metadata?.version !== null
            ? `${metadata.version}`
            : null;
    const versionLabel =
        rawVersionName && versionNumber
            ? `${rawVersionName} (${versionNumber})`
            : rawVersionName ?? versionNumber ?? '0.1.0';
    const releaseVersion = rawVersionName ?? versionNumber ?? '0.1.0';

    const versionButton = new Gtk.Button({
        label: versionLabel,
        halign: Gtk.Align.CENTER,
        margin_top: 4,
        tooltip_text: gettextFunc('Changelog & Releases'),
    });
    versionButton.add_css_class('pill');
    versionButton.add_css_class('kuhlerprofil-version-pill');

    const baseUrl = metadata?.url ?? 'https://github.com/katon26/kuhler-profil';
    const normalizedBaseUrl = baseUrl.replace(/\/$/, '');
    const releasesBaseUrl = normalizedBaseUrl.endsWith('/releases')
        ? normalizedBaseUrl
        : `${normalizedBaseUrl}/releases`;

    versionButton.connect('clicked', () => {
        let targetUrl = releasesBaseUrl;
        if (releaseVersion && releaseVersion !== 'Unknown') {
            const safeVersion = encodeURIComponent(releaseVersion.replace(/^v/, ''));
            targetUrl = `${releasesBaseUrl}/tag/v${safeVersion}`;
        }
        launchUri(window, targetUrl);
    });

    headerBox.append(versionButton);
    headerGroup.add(headerBox);
    aboutPage.add(headerGroup);

    // Two-column Content Box: Links (Left) and QR + Sponsor (Right)
    const contentGroup = new Adw.PreferencesGroup();
    const contentBox = new Gtk.Box({
        orientation: Gtk.Orientation.HORIZONTAL,
        spacing: 24,
        margin_top: 8,
        margin_bottom: 16,
        margin_start: 16,
        margin_end: 16,
        hexpand: true,
        homogeneous: true,
    });

    const leftColumn = new Gtk.Box({
        orientation: Gtk.Orientation.VERTICAL,
        spacing: 12,
        hexpand: true,
        halign: Gtk.Align.FILL,
    });

    const websiteCard = new Adw.PreferencesGroup();
    websiteCard.add(createLinkRow(window, gettextFunc('Website'), normalizedBaseUrl));
    leftColumn.append(websiteCard);

    const issueCard = new Adw.PreferencesGroup();
    issueCard.add(createLinkRow(window, gettextFunc('Report an Issue'), `${normalizedBaseUrl}/issues`));
    leftColumn.append(issueCard);

    const infoGroup = new Adw.PreferencesGroup();
    infoGroup.add(createLinkRow(window, gettextFunc('Credits'), `${normalizedBaseUrl}/graphs/contributors`));
    infoGroup.add(createLegalRow(window, normalizedBaseUrl, gettextFunc));
    leftColumn.append(infoGroup);

    contentBox.append(leftColumn);

    const rightColumn = new Gtk.Box({
        orientation: Gtk.Orientation.VERTICAL,
        spacing: 12,
        halign: Gtk.Align.FILL,
        valign: Gtk.Align.START,
        margin_top: 35,
        hexpand: true,
    });

    // QR Code Button
    const qrButton = new Gtk.Button({
        halign: Gtk.Align.CENTER,
        tooltip_text: gettextFunc('Buy Me a Coffee'),
    });
    qrButton.add_css_class('flat');
    const qrFile = Gio.File.new_for_path(`${extensionPath}/src/qr-code-fuhg.svg`);
    const qrImage = new Gtk.Image({
        gicon: new Gio.FileIcon({ file: qrFile }),
        pixel_size: 128,
    });
    qrButton.set_child(qrImage);
    qrButton.connect('clicked', () => {
        launchUri(window, 'https://buymeacoffee.com/fuhg');
    });
    const qrBox = new Gtk.Box({
        halign: Gtk.Align.CENTER,
        valign: Gtk.Align.CENTER,
        margin_bottom: 12,
    });
    qrBox.append(qrButton);
    rightColumn.append(qrBox);

    // Sponsor Button
    const sponsorButton = new Gtk.Button({
        halign: Gtk.Align.CENTER,
        tooltip_text: gettextFunc('Become a sponsor on GitHub'),
    });
    sponsorButton.add_css_class('pill');
    sponsorButton.add_css_class('kuhlerprofil-coffee-button');

    const sponsorContent = new Gtk.Box({
        orientation: Gtk.Orientation.HORIZONTAL,
        spacing: 8,
    });
    sponsorContent.append(new Gtk.Image({
        gicon: new Gio.FileIcon({ file: Gio.File.new_for_path(`${extensionPath}/src/github-symbolic.svg`) }),
    }));
    sponsorContent.append(new Gtk.Label({
        label: gettextFunc('Sponsor Me ♡'),
    }));
    sponsorButton.set_child(sponsorContent);
    sponsorButton.connect('clicked', () => {
        launchUri(window, 'https://github.com/sponsors/katon26');
    });
    rightColumn.append(sponsorButton);

    contentBox.append(rightColumn);
    contentGroup.add(contentBox);
    aboutPage.add(contentGroup);

    // Responsive breakpoint: stack columns vertically on narrow window widths
    const aboutBreakpoint = new Adw.Breakpoint({
        condition: Adw.BreakpointCondition.parse('max-width: 500sp'),
    });
    aboutBreakpoint.add_setter(contentBox, 'orientation', Gtk.Orientation.VERTICAL);
    aboutBreakpoint.add_setter(contentBox, 'homogeneous', false);
    aboutBreakpoint.add_setter(rightColumn, 'margin-top', 0);
    window.add_breakpoint(aboutBreakpoint);

    return aboutPage;
}

export default class KuhlerProfilPreferences extends ExtensionPreferences {
    fillPreferencesWindow(window) {
        try {
            this.initTranslations();
        } catch (_) {}

        this._ensureCustomCss(window);

        window.set_default_size(680, 750);

        const ProxyWrapper = Gio.DBusProxy.makeProxyWrapper(KuhlerProfilIfaceXML);
        let proxy = null;
        let signalIds = [];
        const bus = getSafeBus();

        // --- PAGE 1: About ---
        const aboutPage = createAboutPage(window, this.path, this.metadata, safeTranslate);
        window.add(aboutPage);

        // --- PAGE 2: Thermal & Cooling Governance ---
        const thermalPage = new Adw.PreferencesPage({
            title: safeTranslate('Thermal & Cooling'),
            icon_name: 'power-profile-balanced-symbolic',
        });
        window.add(thermalPage);

        // Status Banner Group
        const bannerGroup = new Adw.PreferencesGroup();
        const banner = new Adw.Banner({
            title: safeTranslate('Connecting to KühlerProfil daemon (kuhlerprofild)...'),
            revealed: true,
        });
        bannerGroup.add(banner);
        thermalPage.add(bannerGroup);

        // Live Telemetry Group
        const telemGroup = new Adw.PreferencesGroup({
            title: safeTranslate('Live Telemetry & Diagnostics'),
            description: safeTranslate('Real-time sensor metrics read directly from ASUS EC registers.'),
        });
        thermalPage.add(telemGroup);

        const tempRow = new Adw.ActionRow({
            title: safeTranslate('CPU Temperature'),
            subtitle: '-- °C',
        });
        telemGroup.add(tempRow);

        const fanRow = new Adw.ActionRow({
            title: safeTranslate('Cooling Fans (CPU / GPU)'),
            subtitle: '-- RPM',
        });
        telemGroup.add(fanRow);

        // Hardware Profile Group
        const profileGroup = new Adw.PreferencesGroup({
            title: safeTranslate('ASUS Hardware Thermal Profiles'),
            description: safeTranslate('Controls motherboard Embedded Controller (EC) thermal power limits and fan acoustic curves.'),
        });
        thermalPage.add(profileGroup);

        const modeRow = new Adw.ComboRow({
            title: safeTranslate('Active Thermal Mode'),
            subtitle: safeTranslate('Hardware cooling tables hardcoded in laptop EC firmware'),
            model: new Gtk.StringList({
                strings: [
                    safeTranslate('Silent (Whisper quiet, ~2000–2400 RPM)'),
                    safeTranslate('Standard (Balanced everyday cooling, ~2800–3500 RPM)'),
                    safeTranslate('Boost (Maximum performance cooling, ~4200–5200+ RPM)'),
                ],
            }),
        });
        profileGroup.add(modeRow);

        // Dynamic Auto-Governor Group
        const govGroup = new Adw.PreferencesGroup({
            title: safeTranslate('Intelligent Dynamic Governor'),
            description: safeTranslate('Automated temperature regulation shifting ASUS hardware modes smoothly using customizable thermal curves.'),
        });
        thermalPage.add(govGroup);

        const autoSwitchRow = new Adw.SwitchRow({
            title: safeTranslate('Enable Dynamic Auto-Governor'),
            subtitle: safeTranslate('Continuously monitor CPU thermals and shift cooling gears with asymmetric anti-flutter hysteresis'),
        });
        govGroup.add(autoSwitchRow);

        const curveRow = new Adw.ComboRow({
            title: safeTranslate('Target Curve Profile'),
            subtitle: safeTranslate('Acoustic priority vs cooling headroom target thresholds'),
            model: new Gtk.StringList({
                strings: [
                    safeTranslate('Quiet (Acoustic priority, silent up to 75°C)'),
                    safeTranslate('Balanced (Default everyday, smooth 50°C–65°C scaling)'),
                    safeTranslate('Aggressive (Performance priority, early boost ramp at 50°C)'),
                ],
            }),
        });
        govGroup.add(curveRow);

        // Zero-RPM Cooldown Group
        const cooldownGroup = new Adw.PreferencesGroup({
            title: safeTranslate('Zero-RPM Cooldown Governor'),
            description: safeTranslate('Bypasses factory firmware 1-3 minute cooldown latency when temperatures drop below 47°C.'),
        });
        thermalPage.add(cooldownGroup);

        const cooldownRow = new Adw.ComboRow({
            title: safeTranslate('Cooldown Recovery Mode'),
            subtitle: safeTranslate('Behavior after CPU finishes workload and cools to safe idle thermals'),
            model: new Gtk.StringList({
                strings: [
                    safeTranslate('Kick (Instant Reset — re-evaluates curve immediately in 5–10s)'),
                    safeTranslate('Decay (Smart Passive — 15s sustained cool observation before 0 RPM)'),
                    safeTranslate('Off (Factory Default — standard 1–3 minute BIOS firmware timer)'),
                ],
            }),
        });
        cooldownGroup.add(cooldownRow);

        // --- PAGE 3: Battery & System ---
        const batteryPage = new Adw.PreferencesPage({
            title: safeTranslate('Battery & System'),
            icon_name: 'battery-symbolic',
        });
        window.add(batteryPage);

        // Battery Care Group
        const batteryGroup = new Adw.PreferencesGroup({
            title: safeTranslate('Battery Health Charging Guardian'),
            description: safeTranslate('Hardware charge limiter to prolong Lithium-Ion lifespan and prevent heat degradation on AC power.'),
        });
        batteryPage.add(batteryGroup);

        const batteryLimitRow = new Adw.ComboRow({
            title: safeTranslate('Charge End Limit'),
            subtitle: safeTranslate('Hardware stops charging when battery reaches this threshold'),
            model: new Gtk.StringList({
                strings: [
                    safeTranslate('60% (Maximum Lifespan — recommended for stationary AC desk use)'),
                    safeTranslate('80% (Balanced Health — excellent longevity with good mobility)'),
                    safeTranslate('100% (Maximum Capacity — full battery for travel / field work)'),
                ],
            }),
        });
        batteryGroup.add(batteryLimitRow);

        // System Daemon Status Group
        const systemGroup = new Adw.PreferencesGroup({
            title: safeTranslate('System Integration & IPC'),
            description: safeTranslate('Inter-process communication details with the privileged background daemon.'),
        });
        batteryPage.add(systemGroup);

        const dbusRow = new Adw.ActionRow({
            title: safeTranslate('D-Bus Service Interface'),
            subtitle: DBUS_NAME,
        });
        systemGroup.add(dbusRow);

        const cliRow = new Adw.ActionRow({
            title: safeTranslate('Command Line Client'),
            subtitle: safeTranslate('Run "kp status" or "kp cooldown" in terminal for instant CLI control'),
        });
        systemGroup.add(cliRow);

        // --- D-Bus State Sync Logic ---
        let syncing = false;
        const modesMap = ['silent', 'standard', 'boost'];
        const curvesMap = ['quiet', 'balanced', 'aggressive'];
        const cooldownsMap = ['kick', 'decay', 'off'];
        const batteryLimitsMap = [60, 80, 100];

        const syncFromTelemetry = (telem) => {
            if (!telem) return;
            syncing = true;
            try {
                // Mode
                const mIdx = modesMap.indexOf(String(telem.active_mode ?? '').toLowerCase());
                if (mIdx >= 0) modeRow.selected = mIdx;

                // Auto
                autoSwitchRow.active = Boolean(telem.auto_mode ?? false);

                // Curve
                const cIdx = curvesMap.indexOf(String(telem.active_curve_profile ?? telem.active_curve ?? '').toLowerCase());
                if (cIdx >= 0) curveRow.selected = cIdx;

                // Cooldown
                const cdIdx = cooldownsMap.indexOf(String(telem.cooldown_mode ?? '').toLowerCase());
                if (cdIdx >= 0) cooldownRow.selected = cdIdx;

                // Battery Limit
                const bVal = Number(telem.battery_limit ?? 80);
                const bIdx = batteryLimitsMap.indexOf(bVal);
                if (bIdx >= 0) batteryLimitRow.selected = bIdx;

                // Live Metrics
                if (telem.cpu_temp !== undefined) {
                    const temp = Number(telem.cpu_temp);
                    tempRow.subtitle = temp > 0 ? `${temp.toFixed(1)} °C` : safeTranslate('Sensor Idle');
                }
                if (telem.fan1_rpm !== undefined) {
                    const fan1 = Number(telem.fan1_rpm);
                    const fan2 = Number(telem.fan2_rpm ?? 0);
                    const fan1Str = fan1 > 0 ? `${fan1.toLocaleString()} RPM` : safeTranslate('0 RPM (Stopped)');
                    const fan2Str = fan2 > 0 ? `${fan2.toLocaleString()} RPM` : '';
                    fanRow.subtitle = fan2Str ? `CPU: ${fan1Str} | GPU: ${fan2Str}` : fan1Str;
                }

                banner.revealed = false;
            } finally {
                syncing = false;
            }
        };

        const cleanupSignals = () => {
            if (proxy && signalIds.length > 0) {
                for (const id of signalIds) {
                    try {
                        if (typeof proxy.disconnectSignal === 'function') {
                            proxy.disconnectSignal(id);
                        } else if (typeof proxy.disconnect === 'function') {
                            proxy.disconnect(id);
                        }
                    } catch (_) {}
                }
                signalIds = [];
            }
        };

        const setupSignals = () => {
            cleanupSignals();
            if (!proxy) return;

            try {
                const s1 = proxy.connectSignal('ThermalModeChanged', (_p, _s, [mode]) => {
                    const idx = modesMap.indexOf(String(mode).toLowerCase());
                    if (idx >= 0 && !syncing) {
                        syncing = true;
                        try { modeRow.selected = idx; } finally { syncing = false; }
                    }
                });
                if (s1) signalIds.push(s1);

                const s2 = proxy.connectSignal('BatteryLimitChanged', (_p, _s, [limit]) => {
                    const idx = batteryLimitsMap.indexOf(Number(limit));
                    if (idx >= 0 && !syncing) {
                        syncing = true;
                        try { batteryLimitRow.selected = idx; } finally { syncing = false; }
                    }
                });
                if (s2) signalIds.push(s2);

                const s3 = proxy.connectSignal('CurveProfileChanged', (_p, _s, [profile]) => {
                    const idx = curvesMap.indexOf(String(profile).toLowerCase());
                    if (idx >= 0 && !syncing) {
                        syncing = true;
                        try { curveRow.selected = idx; } finally { syncing = false; }
                    }
                });
                if (s3) signalIds.push(s3);

                const s4 = proxy.connectSignal('CooldownModeChanged', (_p, _s, [mode]) => {
                    const idx = cooldownsMap.indexOf(String(mode).toLowerCase());
                    if (idx >= 0 && !syncing) {
                        syncing = true;
                        try { cooldownRow.selected = idx; } finally { syncing = false; }
                    }
                });
                if (s4) signalIds.push(s4);

                const s5 = proxy.connectSignal('TelemetryTick', (_p, _s, [status]) => {
                    syncFromTelemetry(unwrapVariant(status));
                });
                if (s5) signalIds.push(s5);
            } catch (err) {
                console.error(`[KühlerProfil Prefs] Error connecting signals: ${err}`);
            }
        };

        const connectProxy = () => {
            if (!bus) {
                banner.title = safeTranslate('System D-Bus unavailable. Is dbus daemon running?');
                banner.revealed = true;
                return;
            }

            try {
                proxy = new ProxyWrapper(bus, DBUS_NAME, DBUS_PATH, (initProxy, err) => {
                    if (err) {
                        banner.title = safeTranslate('Failed to connect to kuhlerprofild. Is the daemon active?');
                        banner.revealed = true;
                        return;
                    }

                    proxy = initProxy;
                    setupSignals();

                    proxy.GetStatusRemote((result, getErr) => {
                        if (getErr) {
                            banner.title = safeTranslate('Daemon unreachable over D-Bus');
                            banner.revealed = true;
                            return;
                        }
                        syncFromTelemetry(unwrapVariant(result));
                    });
                });
            } catch (err) {
                banner.title = safeTranslate('Failed to initialize D-Bus proxy wrapper');
                banner.revealed = true;
            }
        };

        connectProxy();

        // Connect UI change listeners to send D-Bus calls
        modeRow.connect('notify::selected', () => {
            if (syncing || !proxy) return;
            const mode = modesMap[modeRow.selected] ?? 'standard';
            proxy.SetThermalModeRemote(mode, (_r, err) => {
                if (err) console.error(`[KühlerProfil Prefs] SetThermalMode error: ${err}`);
            });
        });

        autoSwitchRow.connect('notify::active', () => {
            if (syncing || !proxy) return;
            proxy.SetAutoModeRemote(autoSwitchRow.active, (_r, err) => {
                if (err) console.error(`[KühlerProfil Prefs] SetAutoMode error: ${err}`);
            });
        });

        curveRow.connect('notify::selected', () => {
            if (syncing || !proxy) return;
            const curve = curvesMap[curveRow.selected] ?? 'balanced';
            proxy.SetCurveProfileRemote(curve, (_r, err) => {
                if (err) console.error(`[KühlerProfil Prefs] SetCurveProfile error: ${err}`);
            });
        });

        cooldownRow.connect('notify::selected', () => {
            if (syncing || !proxy) return;
            const cd = cooldownsMap[cooldownRow.selected] ?? 'kick';
            proxy.SetCooldownModeRemote(cd, (_r, err) => {
                if (err) console.error(`[KühlerProfil Prefs] SetCooldownMode error: ${err}`);
            });
        });

        batteryLimitRow.connect('notify::selected', () => {
            if (syncing || !proxy) return;
            const limit = batteryLimitsMap[batteryLimitRow.selected] ?? 80;
            proxy.SetBatteryLimitRemote(limit, (_r, err) => {
                if (err) console.error(`[KühlerProfil Prefs] SetBatteryLimit error: ${err}`);
            });
        });

        // Clean up signals when preferences window closes
        window.connect('close-request', () => {
            cleanupSignals();
            proxy = null;
        });
    }

    _ensureCustomCss(window) {
        if (window._kuhlerprofilCssProvider)
            return;

        const cssProvider = new Gtk.CssProvider();
        const cssPath = GLib.build_filenamev([this.path, 'prefs.css']);
        const cssFile = Gio.File.new_for_path(cssPath);

        if (cssFile.query_exists(null)) {
            cssProvider.load_from_path(cssPath);
            const display = Gdk.Display.get_default();
            if (display) {
                Gtk.StyleContext.add_provider_for_display(
                    display,
                    cssProvider,
                    Gtk.STYLE_PROVIDER_PRIORITY_APPLICATION
                );
            }
            window._kuhlerprofilCssProvider = cssProvider;
        }
    }
}
