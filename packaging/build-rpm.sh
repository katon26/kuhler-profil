#!/usr/bin/env bash
# Build RPM package for KühlerProfil (Fedora / RHEL / openSUSE)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION="0.1.0"
NAME="kuhlerprofil"

echo "==> Preparing KühlerProfil RPM packaging..."

RPMBUILD_DIR="${ROOT_DIR}/dist/rpmbuild"
rm -rf "${RPMBUILD_DIR}"
mkdir -p "${RPMBUILD_DIR}"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

# Create source archive
ARCHIVE_NAME="${NAME}-${VERSION}"
TMP_SOURCE="${RPMBUILD_DIR}/${ARCHIVE_NAME}"
mkdir -p "${TMP_SOURCE}"
git -C "${ROOT_DIR}" archive --format=tar --prefix="${ARCHIVE_NAME}/" HEAD | tar -xf - -C "${RPMBUILD_DIR}"
(cd "${RPMBUILD_DIR}" && tar -czf "SOURCES/${ARCHIVE_NAME}.tar.gz" "${ARCHIVE_NAME}")
rm -rf "${TMP_SOURCE}"

cp "${SCRIPT_DIR}/rpm/kuhlerprofil.spec" "${RPMBUILD_DIR}/SPECS/"

if command -v rpmbuild >/dev/null 2>&1; then
    echo "Running rpmbuild..."
    rpmbuild --define "_topdir ${RPMBUILD_DIR}" -ba "${RPMBUILD_DIR}/SPECS/kuhlerprofil.spec"
    cp "${RPMBUILD_DIR}"/RPMS/*/*.rpm "${ROOT_DIR}/dist/" || true
    echo "✔ Built RPM in ${ROOT_DIR}/dist/"
else
    echo "Notice: rpmbuild command not found on this host."
    echo "RPM spec file and source tarball have been staged in ${RPMBUILD_DIR}/"
    echo "To build on Fedora/RHEL:"
    echo "  sudo dnf install -y rpm-build"
    echo "  rpmbuild -ba packaging/rpm/kuhlerprofil.spec"
fi
