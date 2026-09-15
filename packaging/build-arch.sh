#!/usr/bin/env bash
# Prepare & validate Arch Linux package for KühlerProfil

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
ARCH_DIR="${SCRIPT_DIR}/arch"

echo "==> Preparing Arch Linux packaging in ${ARCH_DIR}..."

if command -v namcap >/dev/null 2>&1; then
    echo "Running namcap lint on PKGBUILD..."
    (cd "${ARCH_DIR}" && namcap PKGBUILD)
fi

if command -v makepkg >/dev/null 2>&1; then
    echo "Building package with makepkg..."
    (cd "${ARCH_DIR}" && makepkg -f)
    cp "${ARCH_DIR}"/*.pkg.tar.* "${ROOT_DIR}/dist/" 2>/dev/null || true
    echo "✔ Built Arch package in ${ROOT_DIR}/dist/"
else
    echo "Notice: makepkg not installed on this host."
    echo "PKGBUILD is ready for Arch Linux / AUR submission in packaging/arch/PKGBUILD"
fi
