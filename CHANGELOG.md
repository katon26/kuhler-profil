# Changelog

All notable changes to the **KühlerProfil** project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.1.0] - 2026-09-15

### Major Architectural Highlights

* **Privileged Hardware Daemon (`kuhlerprofild`) & D-Bus IPC**:
  * Runs as a secure systemd system daemon (`kuhlerprofil.service`) communicating over system D-Bus (`org.freedesktop.kuhlerprofil`).
  * Direct kernel sysfs manipulation for ASUS ACPI platform profiles (`throttle_thermal_policy`, `fan_boost_mode`, `platform_profile`).
  * Non-destructive hardware probing (`-dry-run` flag) and live telemetry broadcasting via D-Bus signals (`TelemetryTick`, `ThermalModeChanged`, `BatteryLimitChanged`, `CurveProfileChanged`, `CooldownModeChanged`).

* **Dual-Path Multi-Point Fan Curve Architecture**:
  * **Path A (Hardware ACPI Curves)**: Detects ASUS sysfs custom curve registers (`fan1_custom_point*`, `fan2_custom_point*`) and commits 8-point temperature-to-PWM tables directly to the Embedded Controller (EC).
  * **Path B (Dynamic Software Auto-Governor)**: For laptops without direct sysfs PWM curve registers, an intelligent software governor shifts hardware thermal modes (`Silent`, `Standard`, `Boost`) using customizable curve thresholds:
    * *Quiet*: Acoustic priority, holds whisper-quiet operation up to 75°C.
    * *Balanced*: Default everyday profile, smooth scaling across 50°C–65°C.
    * *Aggressive*: Performance priority, early boost ramp at 50°C.
  * 3-second anti-flutter dwell timing on cooling down-steps and asymmetric hysteresis deadbands prevent rapid fan oscillation.

* **Zero-RPM Cooldown Recovery Governor**:
  * Solves the ASUS EC firmware quirk where fans linger at ~2100–2200 RPM for 1–3 minutes after the CPU cools down to idle temperatures:
    * **`kick` (Thermal Policy "Kick" - Instant Reset)**: Automatically re-asserts sysfs thermal policy once CPU temperature drops $\le 47^\circ\text{C}$ with spinning fans, prompting the EC to release the fan latch in 5–10 seconds (with 10-second debounce).
    * **`decay` (Smart Passive Governor - Cooldown Decay)**: Monitors cooldown trends and releases the fan latch once the CPU idles $\le 47^\circ\text{C}$ continuously for 15 seconds.
    * **`off` (Factory Firmware Default)**: Complete software hands-off, letting factory BIOS timers run unassisted.

* **Battery Health Charging Guardian**:
  * Hardware charging threshold limiter (`charge_control_end_threshold`) with 60%, 80%, and 100% capacity stops to prevent Lithium-Ion degradation and heat soak on continuous AC power.

* **Triple-Interface User Experience**:
  * **Unified CLI (`kuhlerprofil` / `kp` / `kuhler`)**: Intuitive Cobra CLI with human-readable dashboard and machine-readable JSON output (`--json`).
  * **Interactive Terminal TUI**: Bubbletea terminal dashboard with ASCII/Unicode gauges, live telemetry, and single-key shortcuts (`[1/2/3]` mode, `[b]` battery, `[a]` auto governor, `[c]` cooldown).
  * **GNOME Shell 45–50 Quick Settings Extension**: Full-featured quick settings indicator and toggle with live metrics, quick buttons, Orca screen reader accessibility, and Libadwaita Preferences dialog.

* **Redesigned Libadwaita Extension Preferences (`prefs.js`)**:
  * **About Page**: Centered branding header with SVG logo (`src/kuhlerprofil-logo.svg`), version badge, project links, Buy Me a Coffee QR button (`src/qr-code-fuhg.svg`), and GitHub Sponsor button (`src/github-symbolic.svg`).
  * **Thermal & Cooling Page**: Live sensor telemetry, ASUS hardware thermal mode picker, dynamic auto-governor toggle, curve profile selection, and zero-RPM cooldown recovery selector.
  * **Battery & System Page**: Battery charge threshold limit selector and D-Bus IPC daemon status diagnostics.
  * **Custom Styling (`prefs.css`)**: Dynamic CSS provider injection for pill buttons and responsive breakpoint for narrow displays (`< 500sp`).

---

### Added

* **Core & ACPI Driver (`pkg/models/`, `pkg/driver/`, `pkg/config/`)**:
  * `CurvePoint`, `CurveProfile`, `ValidateCurvePoints`, and `ExpandPointsTo8` model validators.
  * Sysfs ACPI drivers with multi-interface fallbacks (`asus-nb-wmi`, `asus_wmi`, `asus_fan_curve`).
  * Hwmon scanner with multi-sensor probing and dual-fan support (CPU & GPU).
  * TOML configuration management with fallback paths (`/etc/kuhlerprofil/config.toml`, `~/.config/kuhlerprofil/config.toml`).

* **Governor & D-Bus IPC (`pkg/engine/`, `pkg/dbusapi/`)**:
  * Thread-safe background governor engine with anti-flutter hysteresis loop.
  * D-Bus server exposing typed methods, signals, and XML introspection.
  * D-Bus client library with mockable interfaces for CLI, TUI, and tests.

* **CLI Commands (`cmd/kuhlerprofil/`)**:
  * `kp status [--json]`: Live telemetry overview.
  * `kp mode [silent|standard|boost]`: Direct thermal mode switching.
  * `kp curve [list|set <profile>]`: Curve profile inspection and activation.
  * `kp cooldown [kick|decay|off]`: Zero-RPM cooldown recovery selection.
  * `kp battery [60|80|100]`: Battery charge limit setting.
  * `kp auto [on|off] [--profile <name>]`: Automatic governor toggle.
  * `kp set <target>`: Smart convenience router.
  * `kp tui`: Launches Bubbletea terminal dashboard.

* **Automated Testing Suite**:
  * Unit tests with Go data race detector (`make test-race`).
  * Comprehensive GNOME Shell extension test suite (`make test-extension`) with 23 automated tests covering D-Bus contracts, HIG guidelines, and Libadwaita UI components.

* **Multi-Platform Packaging Suite (`packaging/`)**:
  * **Debian / Ubuntu**: `packaging/debian/control`, maintainer scripts (`postinst`, `prerm`, `postrm`), and `packaging/build-deb.sh` producing standalone `.deb` binary package.
  * **Fedora / RHEL / openSUSE**: RPM spec file `packaging/rpm/kuhlerprofil.spec` and `packaging/build-rpm.sh`.
  * **Arch Linux / AUR**: `packaging/arch/PKGBUILD`, `packaging/arch/kuhlerprofil.install`, and `packaging/build-arch.sh`.
  * **Makefile Packaging Targets**: `make package-deb`, `make package-rpm`, `make package-arch`, and `make package-all`.

