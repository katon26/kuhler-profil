#!/usr/bin/env bash
# Master packaging script for KühlerProfil (Debian/Ubuntu, Fedora/RPM, Arch Linux, GNOME Extension)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=============================================="
echo "  KühlerProfil Multi-Platform Packaging Suite "
echo "=============================================="

mkdir -p "${ROOT_DIR}/dist"

# 1. Debian/Ubuntu .deb
echo ""
"${SCRIPT_DIR}/build-deb.sh"

# 2. Fedora/RPM
echo ""
"${SCRIPT_DIR}/build-rpm.sh"

# 3. Arch Linux
echo ""
"${SCRIPT_DIR}/build-arch.sh"

# 4. GNOME Shell Extension
echo ""
echo "==> Packaging GNOME Shell Extension..."
"${ROOT_DIR}/extension/install.sh" pack
cp "${ROOT_DIR}/extension/"*.shell-extension.zip "${ROOT_DIR}/dist/" 2>/dev/null || true

echo ""
echo "=============================================="
echo "Packaging artifacts generated in dist/:"
ls -lh "${ROOT_DIR}/dist"
echo "=============================================="
