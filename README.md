# KühlerProfil

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![GNOME Shell](https://img.shields.io/badge/GNOME%20Shell-45%20--%2050-4a86cf?style=flat&logo=gnome)](https://extensions.gnome.org)
[![Linux D-Bus](https://img.shields.io/badge/IPC-Linux%20D--Bus-red?style=flat)](https://www.freedesktop.org/wiki/Software/dbus/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Tests](https://img.shields.io/badge/Tests-100%25%20Passing-brightgreen.svg)]()

> **High-Performance ASUS VivoBook & ZenBook Thermal, Dynamic Fan Curve, and Battery Care Management Suite for Linux.**

KühlerProfil is a lightweight, zero-bloat system utility tailored specifically for ASUS laptops running Linux. Written entirely in pure Go with a companion GNOME Shell 45+ Quick Settings extension, KühlerProfil gives you seamless control over ASUS hardware thermal profiles, dual-fan RPM monitoring, automated thermal curve hysteresis, and hardware battery charge health limiting (60% / 80% / 100%).

---

## Table of Contents

- [Key Features](#key-features)
- [Architecture Overview](#architecture-overview)
- [Hardware Safety & Thermal Architecture](#hardware-safety--thermal-architecture)
- [Supported Hardware & Models](#supported-hardware--models)
- [Quick Start & Installation](#quick-start--installation)
  - [Prerequisites](#prerequisites)
  - [Build and Install Suite](#build-and-install-suite)
  - [Install GNOME Shell Extension](#install-gnome-shell-extension)
- [CLI Reference](#cli-reference)
- [Interactive TUI Dashboard](#interactive-tui-dashboard)
- [GNOME Shell Quick Settings Extension](#gnome-shell-quick-settings-extension)
- [Configuration Reference](#configuration-reference)
- [D-Bus IPC API Specification](#d-bus-ipc-api-specification)
- [Troubleshooting & Diagnostics](#troubleshooting--diagnostics)
- [Contributing](#contributing)
- [License](#license)

---

## Key Features

- **Hardware Thermal Profiles**: Instant switching between ASUS hardware modes (`Silent`, `Standard`, `Boost`) via `asus-nb-wmi` sysfs and Linux ACPI platform profiles.
- **ASUS Battery Care Health Limiter**: Protect lithium-ion health by locking battery charge threshold to **60%**, **80%**, or **100%** (`charge_control_end_threshold`).
- **Dynamic Auto-Governor**: Intelligent hysteresis thermal governor that steps thermal profiles up under sustained load and safely steps them down when cool, eliminating noisy fan hunting.
- **Real-time Dual Fan & CPU Telemetry**: Non-blocking asynchronous sampling of CPU package temperature and primary/secondary fan RPMs via Linux `hwmon`.
- **Interactive Bubbletea TUI**: Gorgeous terminal dashboard with real-time ASCII telemetry gauges, battery status bars, and responsive single-key controls.
- **GNOME Shell 45+ Quick Settings**: Native GNOME top-bar integration with live telemetry subtitle, symbolic icons, and quick-toggle menu cards.
- **Security-Hardened D-Bus IPC**: Centralized `kuhlerprofild` daemon running over `org.freedesktop.kuhlerprofil` system bus with polkit/D-Bus security policies allowing unprivileged desktop clients.
- **100% Mockable & Tested**: Zero required external C dependencies with fully mockable sysfs layers for deterministic unit and race testing.

---

## Architecture Overview

```mermaid
flowchart TD
    subgraph Hardware ["Kernel & Sysfs Layer"]
        SYSFS_THROTTLE["/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy"]
        SYSFS_BATTERY["/sys/class/power_supply/BAT0/charge_control_end_threshold"]
        SYSFS_HWMON["/sys/class/hwmon/hwmon* (CPU Temp, Fan 1 & 2 RPM)"]
    end

    subgraph CoreDaemon ["kuhlerprofild Background Service"]
        DRIVER["pkg/driver: Hardware Sysfs Driver"]
        ENGINE["pkg/engine: Telemetry Engine & Auto-Governor"]
        CONFIG["pkg/config: TOML Config Manager"]
        DBUS_SERVER["pkg/dbusapi: D-Bus IPC Server"]

        SYSFS_THROTTLE <--> DRIVER
        SYSFS_BATTERY <--> DRIVER
        SYSFS_HWMON --> DRIVER

        DRIVER <--> ENGINE
        CONFIG <--> ENGINE
        ENGINE <--> DBUS_SERVER
    end

    subgraph Clients ["User Interfaces & Clients"]
        DBUS_BUS["System D-Bus: org.freedesktop.kuhlerprofil"]
        CLI["cmd/kuhlerprofil: Cobra CLI (aliases: kp, kuhler)"]
        TUI["pkg/tui: Bubbletea Terminal Dashboard"]
        GNOME_EXT["extension: GNOME Shell Quick Settings"]

        DBUS_SERVER <==> DBUS_BUS
        DBUS_BUS <==> CLI
        DBUS_BUS <==> TUI
        DBUS_BUS <==> GNOME_EXT
    end
```

---

## Hardware Safety & Thermal Architecture

When managing system thermals, fan curves, and power thresholds on Linux, hardware safety and transparency are paramount. **KühlerProfil is designed to be completely non-destructive and failsafe by design.**

### 1. Silicon & Firmware Hardware Overrides (PROCHOT & EC)
- **No Direct Register or Port Bypasses:** KühlerProfil does **not** write to raw memory registers, raw I/O ports (`/dev/port`), or bypass EC firmware safety trips with forced PWM voltage overrides.
- **Kernel-Standard Driver Communication:** All thermal mode operations interface strictly through the official Linux kernel driver:
  ```
  /sys/devices/platform/asus-nb-wmi/throttle_thermal_policy
  ```
  *(with standard fallbacks to `fan_boost_mode` and ACPI `platform_profile`).*
- **Vendor Thermal Tables:** This interface corresponds directly to ASUS's built-in Embedded Controller (EC) profile tables (equivalent to pressing `Fn + F` on your keyboard or toggling modes in ASUS Armoury Crate / MyASUS on Windows):
  - `0`: Standard / Balanced
  - `1`: Boost / Performance
  - `2`: Silent / Whisper
- **Emergency Hardware Overrides Remain Active:** Modern AMD and Intel ASUS laptops enforce autonomous safety fuses at the silicon level. If CPU/GPU package temperatures spike toward dangerous trip points (e.g., 90°C–95°C), the laptop's hardware EC firmware and CPU hardware thermal trip (**BD PROCHOT**) will **always automatically override** any user-space profile to spin fans to maximum RPM or throttle CPU clocks, preventing heat damage even if KühlerProfil is active in "Silent" mode.

### 2. Battery Health Care Limiter Safety
- **Strict Input Validation:** Writes to `/sys/class/power_supply/BAT*/charge_control_end_threshold` are strictly validated before any sysfs operation occurs. Only the manufacturer-supported thresholds (**60%**, **80%**, and **100%**) are accepted.
- **Hardware-Managed Charging:** This threshold communicates directly with the battery management IC on the motherboard to stop charging when the set capacity is reached. It does not stress the battery cells; keeping lithium-ion batteries capped at 60% or 80% while connected to AC power significantly mitigates degradation and prolongs battery lifespan.

### 3. Failsafe Verification & Diagnostics
Before deploying the background daemon, users can non-destructively probe their system without root privileges using the built-in dry-run diagnostic:
```bash
./bin/kuhlerprofild -dry-run
# or
make dry-run
```
This inspects and reports detected kernel endpoints, hwmon sensor channels, and battery controls before any daemon operations are initiated.

---

## Supported Hardware & Models

KühlerProfil supports ASUS laptops featuring the `asus_nb_wmi` or `asus_wmi` kernel module, as well as laptops supporting Linux ACPI platform profiles:

- **ASUS VivoBook Series**:
  - VivoBook S14 / S15 / S16 (K5404, S5404, M5402, M5502, S533, etc.)
  - VivoBook Pro 14 / 15 / 16 OLED (K3400, K3500, K3502, K6502, N7600)
  - VivoBook Go 14 / 15, Flip 14
  - VivoBook 15 / 17 (X512, X515, M515, X712)
- **ASUS ZenBook Series**:
  - ZenBook 13 / 14 / 15 / Flip / Duo (UX325, UX425, UX434, UX482, UM425, etc.)
  - ZenBook Pro 14 / 16 OLED
- **ASUS ROG & TUF Series** (Thermal mode & battery limiter compatibility):
  - ROG Zephyrus G14, G15, G16, M16
  - ASUS TUF Gaming A15, A16, F15, F17
  - ROG Flow X13, Z13, X16

> **Kernel Requirement**: Linux 5.15+ (Linux 6.2+ recommended for complete multi-fan hwmon support).

---

## Quick Start & Installation

### Prerequisites

Ensure you have the standard build toolchain installed on your Linux distribution:

- **Go**: Version 1.22 or higher
- **Make**: Standard GNU Make
- **D-Bus & systemd**: Standard on Fedora, Ubuntu, Debian, Arch Linux, openSUSE, etc.
- **Node.js** *(optional, for running extension tests)*

#### Fedora / RHEL
```bash
sudo dnf install golang make dbus-devel
```

#### Ubuntu / Debian
```bash
sudo apt update && sudo apt install -y golang make dbus
```

#### Arch Linux
```bash
sudo pacman -S go make
```

---

### Build and Install Suite

```bash
# 1. Clone the repository
git clone https://github.com/asus-linux/kuhler-profil.git
cd kuhler-profil

# 2. Compile both binaries (CLI & Daemon)
make build

# 3. Install daemon, CLI, symlinks (kp, kuhler), systemd service, and D-Bus policy (requires root)
sudo make install

# 4. Reload systemd & D-Bus, then enable the daemon
sudo systemctl daemon-reload
sudo systemctl reload dbus
sudo systemctl enable --now kuhlerprofil.service
```

Verify the daemon is running properly:
```bash
systemctl status kuhlerprofil.service
kp status
# or:
kuhlerprofil status
```

---

### Install GNOME Shell Extension

To install the KühlerProfil Quick Settings menu for GNOME 45, 46, 47, 48+:

```bash
# Install extension to ~/.local/share/gnome-shell/extensions/
make install-extension

# If running Wayland, log out and log back in, or enable directly:
gnome-extensions enable kuhlerprofil@asus-linux.org
```

---

## CLI Reference

The `kuhlerprofil` CLI (and convenient `kp` short alias) provides full control over hardware profiles, battery thresholds, governor settings, and telemetry output.

Both `kuhlerprofil` and `kp` (as well as `kuhler`) are symlinked and can be used interchangeably:

### 1. Show System Status

```bash
# Human-readable dashboard summary
kp status
# or:
kuhlerprofil status

# Machine-readable JSON output (ideal for scripts, Waybar, Polybar, Conky)
kp status --json
```

**JSON Output Example:**
```json
{
  "cpu_temp": 52.4,
  "fan1_rpm": 2400,
  "fan2_rpm": 0,
  "battery_percent": 79,
  "battery_limit": 80,
  "on_ac": true,
  "active_mode": "standard",
  "auto_mode": false
}
```

### 2. Thermal Profile Switching

```bash
# View active profile
kp mode

# Set thermal mode: silent (whisper quiet), standard (balanced), boost (max cooling)
kp mode silent
kp mode standard
kp mode boost
```

### 3. Battery Health Care Limit

```bash
# View active charge limit
kp battery

# Set battery charging limit to 60%, 80%, or 100%
kp battery 60
kp battery 80
kp battery 100
```

### 4. Dynamic Auto-Governor

```bash
# Query auto governor status
kp auto

# Enable / disable dynamic thermal curve governor
kp auto on
kp auto off

# Enable auto governor with a specific curve profile
kp auto on --profile quiet
```

### 5. Fan Curve Profiles

Query or set active temperature-to-fan curve profiles:

```bash
# View active curve profile and hardware ACPI support status
kp curve

# List all available curve profiles
kp curve list

# Switch to a curve profile: quiet, balanced, aggressive, or custom
kp curve set quiet
kp curve set balanced
kp curve set aggressive
```

### 6. Interactive Terminal TUI

```bash
# Launch full-screen interactive dashboard
kp tui
# or simply:
kp
# or:
kuhlerprofil
```

---

## Interactive TUI Dashboard

KühlerProfil includes a responsive terminal UI powered by [Bubbletea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss).

Run:
```bash
kp tui
# or simply:
kp
```

```
┌──────────────────────── KühlerProfil ASUS Control ─────────────────────────┐
│ Profile: [ STANDARD ]     Governor: [ OFF ]             Power: [ AC Power ]│
├────────────────────────────────────────────────────────────────────────────┤
│ CPU Temperature: 54.0°C   [██████████████████░░░░░░░░░░░░░░░░░░░░░░░░]     │
│ Fan 1 (CPU):     2400 RPM [████████████████████████░░░░░░░░░░░░░░░░░░]     │
│ Fan 2 (GPU):     0 RPM    [░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░]     │
│ Battery Health:  78%      [█████████████████████████████████░░░] Limit: 80%│
├────────────────────────────────────────────────────────────────────────────┤
│ [1] Silent    [2] Standard    [3] Boost    [B] Cycle Battery Limit         │
│ [A] Auto-Curve Governor       [R] Refresh  [Q/Esc] Quit                    │
└────────────────────────────────────────────────────────────────────────────┘
```

### Interactive Keybindings

| Key | Action |
|:---:|:---|
| `1` | Switch to **Silent** mode (ASUS Quiet / Whisper profile) |
| `2` | Switch to **Standard** mode (ASUS Balanced profile) |
| `3` | Switch to **Boost** mode (ASUS Performance / Overboost profile) |
| `b` / `B` | Cycle Battery Charge Limit (`60%` ➔ `80%` ➔ `100%`) |
| `a` / `A` | Toggle Dynamic Auto-Governor (`ON` / `OFF`) |
| `r` / `R` | Force immediate telemetry refresh |
| `q` / `Esc` / `Ctrl+C` | Quit TUI |

---

## GNOME Shell Quick Settings Extension

The GNOME Shell extension integrates directly into the Quick Settings menu on GNOME 45, 46, 47, and 48+:

- **Top Bar Indicator**: Displays active profile icon and live subtitle metric (e.g. `Standard · 52°C · 2400 RPM`).
- **Telemetry Cards**: Real-time 2x2 grid showing CPU Temperature, Battery Charge, Fan 1 RPM, and Fan 2 RPM.
- **One-Click Controls**: Toggle Silent / Standard / Boost, set 60% / 80% / 100% battery caps, and toggle auto-governor.
- **Asynchronous D-Bus**: Zero UI freezing, event-driven updates via D-Bus signal subscriptions.

```bash
# Package extension for distribution
make extension-pack

# Install from local directory
make install-extension
```

---

## Configuration Reference

Configuration files are loaded from `/etc/kuhlerprofil/config.toml` (system-wide) or `~/.config/kuhlerprofil/config.toml` (user-level override).

```toml
# /etc/kuhlerprofil/config.toml

# Default ASUS thermal profile on daemon boot: "silent", "standard", or "boost"
default_mode = "standard"

# Default battery charge health limit percentage: 60, 80, or 100
default_battery_limit = 80

# Hardware telemetry sensor polling interval in milliseconds
poll_interval_ms = 1500

# Enable automated dynamic thermal curve hysteresis governor
auto_curve = false

# Thermal governor hysteresis thresholds (Celsius)
hysteresis_step_up_temp = 75.0      # Temperature to step Standard -> Boost
hysteresis_step_down_temp = 55.0    # Temperature to step Boost -> Standard / Silent
```

---

## D-Bus IPC API Specification

The `kuhlerprofild` daemon registers on the Linux System Bus:

- **Service Name**: `org.freedesktop.kuhlerprofil`
- **Object Path**: `/org/freedesktop/kuhlerprofil`
- **Interface**: `org.freedesktop.kuhlerprofil`

### Methods

| Method | Signature | Description |
|:---|:---:|:---|
| `GetStatus` | `() -> (a{sv})` | Returns a dictionary containing all real-time telemetry metrics. |
| `SetThermalMode` | `(s) -> ()` | Sets active thermal mode (`"silent"`, `"standard"`, or `"boost"`). |
| `SetBatteryLimit` | `(i) -> ()` | Sets battery health charging threshold (`60`, `80`, or `100`). |
| `SetAutoMode` | `(b) -> ()` | Enables (`true`) or disables (`false`) dynamic auto-governor. |

### Signals

| Signal | Signature | Description |
|:---|:---:|:---|
| `ThermalModeChanged` | `(s)` | Emitted when thermal mode changes with new mode name. |
| `BatteryLimitChanged` | `(i)` | Emitted when battery charge limit threshold is updated. |
| `TelemetryTick` | `(a{sv})` | Broadcast on every telemetry polling cycle with updated values. |

### Testing with `busctl` or `gdbus`

```bash
# Query status dictionary
busctl call org.freedesktop.kuhlerprofil /org/freedesktop/kuhlerprofil org.freedesktop.kuhlerprofil GetStatus

# Set thermal mode to boost
busctl call org.freedesktop.kuhlerprofil /org/freedesktop/kuhlerprofil org.freedesktop.kuhlerprofil SetThermalMode s "boost"

# Set battery limit to 80%
busctl call org.freedesktop.kuhlerprofil /org/freedesktop/kuhlerprofil org.freedesktop.kuhlerprofil SetBatteryLimit i 80
```

---

## Troubleshooting & Diagnostics

### 1. Hardware Probe Check (`make dry-run`)

Run a capability dry-run to inspect detected sysfs endpoints without needing root privileges:

```bash
make dry-run
# or:
./bin/kuhlerprofild -dry-run
```

**Expected Output:**
```
[kuhlerprofild] Hardware capability probe:
[kuhlerprofild]   • Thermal mode control: true (policy: true, boost: false, profile: false, path: /sys/devices/platform/asus-nb-wmi/throttle_thermal_policy)
[kuhlerprofild]   • Battery charge limiter: true (path: /sys/class/power_supply/BAT0/charge_control_end_threshold)
[kuhlerprofild]   • Telemetry sensors: CPU temp=true, Fan1=true, Fan2=false (hwmon: /sys/class/hwmon/hwmon0)
```

### 2. Verify Kernel Module

Ensure `asus_nb_wmi` or `asus_wmi` is loaded in your Linux kernel:

```bash
lsmod | grep asus
```

If not loaded, check modprobe:
```bash
sudo modprobe asus_nb_wmi
```

### 3. Check D-Bus Policy

Ensure `/etc/dbus-1/system.d/org.freedesktop.kuhlerprofil.conf` exists and D-Bus is reloaded (`sudo systemctl reload dbus`).

---

## Development & Testing

### 1. Automated Software Tests
Run all unit tests and race condition checks:

```bash
# Run unit test suite
make test

# Run Go race detector (checks for concurrency issues)
make test-race

# Run GNOME extension automated tests
make test-extension

# Run complete verification suite (race tests + extension tests)
make test-all
```

### 2. Non-Destructive Hardware Smoke Test
Probe your laptop's real sensors and ACPI interfaces without root or system changes:

```bash
make dry-run
# or
./bin/kuhlerprofild -dry-run
```

### 3. Local Daemon & CLI Testing (Without Installing to System)
To test the daemon and CLI locally in real-time:

```bash
# Terminal 1: Run the daemon in foreground (requires sudo for sysfs access)
sudo ./bin/kuhlerprofild

# Terminal 2: Test commands against the running daemon
./bin/kuhlerprofil status
./bin/kuhlerprofil curve list
./bin/kuhlerprofil curve set quiet
./bin/kuhlerprofil tui
```

---

## Contributing

Contributions are welcome! KühlerProfil is an open-source project designed for Linux laptop enthusiasts.

1. **Fork the repository** on GitHub.
2. **Create a feature branch** (`git checkout -b feature/amazing-feature`).
3. **Commit your changes** (`git commit -m 'feat: my cool feature'`).
4. **Ensure all tests pass** (`make test-all`).
5. **Open a Pull Request**.

### Areas for Contribution:
- [x] Custom multi-point RPM fan curve profiles (Dual-Path ACPI & Software Governor)
- [ ] ROG keyboard RGB backlight integration (`asus::kbd_backlight`)
- [ ] Waybar & Polybar native integration scripts
- [ ] KDE Plasma 6 Quick Settings Widget

---

## License

KühlerProfil is open-source software licensed under the [MIT License](LICENSE).

