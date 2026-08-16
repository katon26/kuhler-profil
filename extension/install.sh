#!/usr/bin/env bash
# KoolThing GNOME Shell Extension Installer
# Installs KoolThing Quick Settings extension to ~/.local/share/gnome-shell/extensions/

set -euo pipefail

UUID="koolthing@asus-linux.org"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EXT_DEST="${HOME}/.local/share/gnome-shell/extensions/${UUID}"

print_usage() {
    echo "Usage: $0 [install|enable|disable|uninstall|pack|status]"
    echo ""
    echo "Commands:"
    echo "  install    Install or update extension files into local user directory (default)"
    echo "  enable     Enable extension using gnome-extensions CLI"
    echo "  disable    Disable extension using gnome-extensions CLI"
    echo "  uninstall  Remove extension from user directory"
    echo "  pack       Package extension into a distributable zip archive"
    echo "  status     Check extension installation and enable status"
}

ACTION="${1:-install}"

case "${ACTION}" in
    install)
        echo "Installing KoolThing GNOME Shell Extension (${UUID})..."
        mkdir -p "${EXT_DEST}"
        cp -v "${SCRIPT_DIR}/metadata.json" "${EXT_DEST}/"
        cp -v "${SCRIPT_DIR}/extension.js" "${EXT_DEST}/"
        cp -v "${SCRIPT_DIR}/stylesheet.css" "${EXT_DEST}/"
        
        echo ""
        echo "Extension successfully installed to ${EXT_DEST}"
        echo "To enable:"
        echo "  gnome-extensions enable ${UUID}"
        echo "Note: If running under X11, you can reload GNOME Shell with Alt+F2 -> 'r' -> Enter."
        echo "      If running under Wayland, log out and log back in to reload GNOME Shell."
        ;;

    enable)
        echo "Enabling ${UUID}..."
        if command -v gnome-extensions >/dev/null 2>&1; then
            gnome-extensions enable "${UUID}"
            echo "KoolThing extension enabled."
        else
            echo "Error: gnome-extensions command not found." >&2
            exit 1
        fi
        ;;

    disable)
        echo "Disabling ${UUID}..."
        if command -v gnome-extensions >/dev/null 2>&1; then
            gnome-extensions disable "${UUID}"
            echo "KoolThing extension disabled."
        else
            echo "Error: gnome-extensions command not found." >&2
            exit 1
        fi
        ;;

    uninstall)
        echo "Uninstalling KoolThing GNOME Shell Extension..."
        if command -v gnome-extensions >/dev/null 2>&1; then
            gnome-extensions disable "${UUID}" 2>/dev/null || true
        fi
        if [ -d "${EXT_DEST}" ]; then
            rm -rf "${EXT_DEST}"
            echo "Removed ${EXT_DEST}"
        fi
        echo "KoolThing extension uninstalled."
        ;;

    pack)
        echo "Packaging ${UUID}.zip..."
        ZIP_NAME="${SCRIPT_DIR}/${UUID}.zip"
        rm -f "${ZIP_NAME}"
        if command -v gnome-extensions >/dev/null 2>&1; then
            gnome-extensions pack "${SCRIPT_DIR}" --force --out-dir="${SCRIPT_DIR}"
            echo "Packaged extension to ${ZIP_NAME}"
        else
            (cd "${SCRIPT_DIR}" && zip -r "${ZIP_NAME}" metadata.json extension.js stylesheet.css)
            echo "Packaged extension to ${ZIP_NAME} (using zip)"
        fi
        ;;

    status)
        echo "=== KoolThing Extension Status ==="
        echo "Target UUID: ${UUID}"
        if [ -d "${EXT_DEST}" ]; then
            echo "Installation: INSTALLED at ${EXT_DEST}"
        else
            echo "Installation: NOT INSTALLED"
        fi

        if command -v gnome-extensions >/dev/null 2>&1; then
            echo -n "GNOME Extensions Status: "
            gnome-extensions info "${UUID}" 2>&1 || true
        fi
        ;;

    -h|--help|help)
        print_usage
        exit 0
        ;;

    *)
        echo "Unknown command: ${ACTION}" >&2
        print_usage
        exit 1
        ;;
esac
