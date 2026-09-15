SHELL := /bin/bash
GO ?= $(shell which go 2>/dev/null || echo /home/kaf/Projects/koolthing/.go_sdk/bin/go)
NODE ?= $(shell which node 2>/dev/null || echo node)

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
SYSTEMD_DIR ?= /etc/systemd/system
DBUS_DIR ?= /etc/dbus-1/system.d
CONFIG_DIR ?= /etc/kuhlerprofil

VERSION ?= 0.1.0
LDFLAGS := -s -w

BIN_DIR := bin
DAEMON_BIN := $(BIN_DIR)/kuhlerprofild
CLI_BIN := $(BIN_DIR)/kuhlerprofil
ALIAS_BIN := $(BIN_DIR)/kp
ALIAS_KUHLER_BIN := $(BIN_DIR)/kuhler

.PHONY: all build daemon cli test test-race test-extension test-all fmt vet install install-daemon install-cli install-extension extension-install extension-pack package-deb package-rpm package-arch package-all uninstall uninstall-extension clean dry-run help

all: build

help:
	@echo "KühlerProfil Build Automation"
	@echo "============================="
	@echo "  make build             Compile both kuhlerprofil CLI and kuhlerprofild daemon"
	@echo "  make test              Run standard unit test suite"
	@echo "  make test-race         Run unit tests with Go data race detector (-race)"
	@echo "  make test-extension    Run GNOME Shell extension automated test suite"
	@echo "  make test-all          Run full Go race tests and JS extension tests"
	@echo "  make dry-run           Probe current hardware capabilities with kuhlerprofild -dry-run"
	@echo "  make install           Install daemon, CLI, systemd unit, and D-Bus policy (requires sudo)"
	@echo "  make install-daemon    Install only kuhlerprofild daemon and system files (requires sudo)"
	@echo "  make install-cli       Install only kuhlerprofil CLI to /usr/local/bin (requires sudo)"
	@echo "  make install-extension Install GNOME Shell 45+ Quick Settings extension to user directory"
	@echo "  make extension-pack    Package extension into distributable ZIP archive"
	@echo "  make package-deb       Build Debian/Ubuntu .deb binary package"
	@echo "  make package-rpm       Prepare RPM package spec and source archive"
	@echo "  make package-arch      Validate and build Arch Linux package using PKGBUILD"
	@echo "  make package-all       Build distribution packages for Debian, RPM, Arch, and GNOME"
	@echo "  make uninstall         Remove binaries, systemd unit, and D-Bus policy"
	@echo "  make clean             Remove built binaries and packaging artifacts"

build: daemon cli

daemon:
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(DAEMON_BIN) ./cmd/kuhlerprofild
	@echo "Built $(DAEMON_BIN)"

cli:
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(CLI_BIN) ./cmd/kuhlerprofil
	@ln -sf kuhlerprofil $(ALIAS_BIN)
	@ln -sf kuhlerprofil $(ALIAS_KUHLER_BIN)
	@echo "Built $(CLI_BIN) with aliases $(ALIAS_BIN) and $(ALIAS_KUHLER_BIN)"

test:
	$(GO) test -v ./...

test-race:
	$(GO) test -v -race ./...

test-extension:
	$(NODE) extension/test/extension_test.js

test-all: test-race test-extension

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

dry-run: build
	./$(DAEMON_BIN) -dry-run

install: install-daemon install-cli
	@echo ""
	@echo "==> Installation complete!"
	@echo "To enable and start the KühlerProfil background service, run:"
	@echo "  sudo systemctl daemon-reload"
	@echo "  sudo systemctl reload dbus"
	@echo "  sudo systemctl enable --now kuhlerprofil.service"

install-daemon: $(DAEMON_BIN)
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 $(DAEMON_BIN) $(DESTDIR)$(BINDIR)/kuhlerprofild
	install -d $(DESTDIR)$(SYSTEMD_DIR)
	install -m 644 systemd/kuhlerprofil.service $(DESTDIR)$(SYSTEMD_DIR)/kuhlerprofil.service
	install -d $(DESTDIR)$(DBUS_DIR)
	install -m 644 systemd/org.freedesktop.kuhlerprofil.conf $(DESTDIR)$(DBUS_DIR)/org.freedesktop.kuhlerprofil.conf
	install -d $(DESTDIR)$(CONFIG_DIR)

install-cli: $(CLI_BIN)
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 $(CLI_BIN) $(DESTDIR)$(BINDIR)/kuhlerprofil
	ln -sf kuhlerprofil $(DESTDIR)$(BINDIR)/kp
	ln -sf kuhlerprofil $(DESTDIR)$(BINDIR)/kuhler

install-extension:
	./extension/install.sh install

extension-install: install-extension

extension-pack:
	./extension/install.sh pack

package-deb:
	./packaging/build-deb.sh

package-rpm:
	./packaging/build-rpm.sh

package-arch:
	./packaging/build-arch.sh

package-all:
	./packaging/build-all.sh

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/kuhlerprofild
	rm -f $(DESTDIR)$(BINDIR)/kuhlerprofil
	rm -f $(DESTDIR)$(BINDIR)/kp
	rm -f $(DESTDIR)$(BINDIR)/kuhler
	rm -f $(DESTDIR)$(SYSTEMD_DIR)/kuhlerprofil.service
	rm -f $(DESTDIR)$(DBUS_DIR)/org.freedesktop.kuhlerprofil.conf
	@echo "Removed kuhlerprofil binaries, systemd unit, and D-Bus configuration."

uninstall-extension:
	./extension/install.sh uninstall

clean:
	rm -rf $(BIN_DIR) dist
	rm -f *.shell-extension.zip *.zip extension/*.zip extension/*.shell-extension.zip
	@echo "Cleaned build artifacts."
