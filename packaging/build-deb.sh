#!/usr/bin/env bash
# Build Debian/Ubuntu .deb package for KühlerProfil

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION="0.1.0"
ARCH="amd64"
PKG_NAME="kuhlerprofil_${VERSION}_${ARCH}"
BUILD_DIR="${ROOT_DIR}/dist/${PKG_NAME}"
OUT_DEB="${ROOT_DIR}/dist/${PKG_NAME}.deb"

echo "==> Building KühlerProfil .deb package (${PKG_NAME}.deb)..."

# 1. Compile binaries if missing
if [ ! -f "${ROOT_DIR}/bin/kuhlerprofild" ] || [ ! -f "${ROOT_DIR}/bin/kuhlerprofil" ]; then
    echo "Compiling binaries with make build..."
    make -C "${ROOT_DIR}" build
fi

# 2. Prepare package staging directory
rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}/DEBIAN"
mkdir -p "${BUILD_DIR}/usr/local/bin"
mkdir -p "${BUILD_DIR}/etc/systemd/system"
mkdir -p "${BUILD_DIR}/etc/dbus-1/system.d"
mkdir -p "${BUILD_DIR}/etc/kuhlerprofil"
mkdir -p "${BUILD_DIR}/usr/share/doc/kuhlerprofil"

# 3. Copy binaries & symlinks
cp "${ROOT_DIR}/bin/kuhlerprofild" "${BUILD_DIR}/usr/local/bin/"
cp "${ROOT_DIR}/bin/kuhlerprofil" "${BUILD_DIR}/usr/local/bin/"
ln -sf kuhlerprofil "${BUILD_DIR}/usr/local/bin/kp"
ln -sf kuhlerprofil "${BUILD_DIR}/usr/local/bin/kuhler"

# 4. Copy system configuration
cp "${ROOT_DIR}/systemd/kuhlerprofil.service" "${BUILD_DIR}/etc/systemd/system/"
cp "${ROOT_DIR}/systemd/org.freedesktop.kuhlerprofil.conf" "${BUILD_DIR}/etc/dbus-1/system.d/"

# 5. Copy documentation & license
cp "${ROOT_DIR}/README.md" "${BUILD_DIR}/usr/share/doc/kuhlerprofil/"
cp "${ROOT_DIR}/LICENSE" "${BUILD_DIR}/usr/share/doc/kuhlerprofil/copyright"

# 6. Copy Debian metadata & maintainer scripts
cp "${SCRIPT_DIR}/debian/control" "${BUILD_DIR}/DEBIAN/"
cp "${SCRIPT_DIR}/debian/postinst" "${BUILD_DIR}/DEBIAN/"
cp "${SCRIPT_DIR}/debian/prerm" "${BUILD_DIR}/DEBIAN/"
cp "${SCRIPT_DIR}/debian/postrm" "${BUILD_DIR}/DEBIAN/"

# Ensure correct permissions
chmod -R 755 "${BUILD_DIR}/usr/local/bin"
chmod 644 "${BUILD_DIR}/etc/systemd/system/kuhlerprofil.service"
chmod 644 "${BUILD_DIR}/etc/dbus-1/system.d/org.freedesktop.kuhlerprofil.conf"
chmod 755 "${BUILD_DIR}/DEBIAN/postinst" "${BUILD_DIR}/DEBIAN/prerm" "${BUILD_DIR}/DEBIAN/postrm"
chmod 644 "${BUILD_DIR}/DEBIAN/control"

# 7. Package using dpkg-deb or fallback to internal ar/tar builder
if command -v dpkg-deb >/dev/null 2>&1; then
    dpkg-deb --build --root-owner-group "${BUILD_DIR}" "${OUT_DEB}"
else
    echo "dpkg-deb not found. Creating standards-compliant .deb via ar/tar..."
    TEMP_PACK="${ROOT_DIR}/dist/.deb_tmp"
    rm -rf "${TEMP_PACK}"
    mkdir -p "${TEMP_PACK}"

    echo "2.0" > "${TEMP_PACK}/debian-binary"
    (cd "${BUILD_DIR}/DEBIAN" && tar --owner=0 --group=0 -czf "${TEMP_PACK}/control.tar.gz" .)
    (cd "${BUILD_DIR}" && tar --owner=0 --group=0 --exclude="./DEBIAN" -czf "${TEMP_PACK}/data.tar.gz" .)
    (cd "${TEMP_PACK}" && ar rcs "${OUT_DEB}" debian-binary control.tar.gz data.tar.gz)
    rm -rf "${TEMP_PACK}"
fi

echo "✔ Built Debian package: ${OUT_DEB}"
