SHELL := /bin/bash
GO ?= $(shell which go 2>/dev/null || echo /home/kaf/.local/go/bin/go)
NODE ?= $(shell which node 2>/dev/null || echo node)

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
SYSTEMD_DIR ?= /etc/systemd/system
DBUS_DIR ?= /etc/dbus-1/system.d
CONFIG_DIR ?= /etc/koolthing

VERSION ?= 0.1.0
LDFLAGS := -s -w

BIN_DIR := bin
DAEMON_BIN := $(BIN_DIR)/koolthingd
CLI_BIN := $(BIN_DIR)/koolthing

.PHONY: all build daemon cli test test-race test-extension test-all fmt vet install install-daemon install-cli install-extension extension-install extension-pack uninstall uninstall-extension clean dry-run help

all: build

help:
	@echo "KoolThing Build Automation"
	@echo "=========================="
	@echo "  make build             Compile both koolthing CLI and koolthingd daemon"
	@echo "  make test              Run standard unit test suite"
	@echo "  make test-race         Run unit tests with Go data race detector (-race)"
	@echo "  make test-extension    Run GNOME Shell extension automated test suite"
	@echo "  make test-all          Run full Go race tests and JS extension tests"
	@echo "  make dry-run           Probe current hardware capabilities with koolthingd -dry-run"
	@echo "  make install           Install daemon, CLI, systemd unit, and D-Bus policy (requires sudo)"
	@echo "  make install-daemon    Install only koolthingd daemon and system files (requires sudo)"
	@echo "  make install-cli       Install only koolthing CLI to /usr/local/bin (requires sudo)"
	@echo "  make install-extension Install GNOME Shell 45+ Quick Settings extension to user directory"
	@echo "  make extension-pack    Package extension into distributable ZIP archive"
	@echo "  make uninstall         Remove binaries, systemd unit, and D-Bus policy"
	@echo "  make clean             Remove built binaries and packaging artifacts"

build: daemon cli

daemon:
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(DAEMON_BIN) ./cmd/koolthingd
	@echo "Built $(DAEMON_BIN)"

cli:
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(CLI_BIN) ./cmd/koolthing
	@echo "Built $(CLI_BIN)"

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
	@echo "To enable and start the KoolThing background service, run:"
	@echo "  sudo systemctl daemon-reload"
	@echo "  sudo systemctl reload dbus"
	@echo "  sudo systemctl enable --now koolthing.service"

install-daemon: $(DAEMON_BIN)
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 $(DAEMON_BIN) $(DESTDIR)$(BINDIR)/koolthingd
	install -d $(DESTDIR)$(SYSTEMD_DIR)
	install -m 644 systemd/koolthing.service $(DESTDIR)$(SYSTEMD_DIR)/koolthing.service
	install -d $(DESTDIR)$(DBUS_DIR)
	install -m 644 systemd/org.freedesktop.koolthing.conf $(DESTDIR)$(DBUS_DIR)/org.freedesktop.koolthing.conf
	install -d $(DESTDIR)$(CONFIG_DIR)

install-cli: $(CLI_BIN)
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 $(CLI_BIN) $(DESTDIR)$(BINDIR)/koolthing

install-extension:
	./extension/install.sh install

extension-install: install-extension

extension-pack:
	./extension/install.sh pack

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/koolthingd
	rm -f $(DESTDIR)$(BINDIR)/koolthing
	rm -f $(DESTDIR)$(SYSTEMD_DIR)/koolthing.service
	rm -f $(DESTDIR)$(DBUS_DIR)/org.freedesktop.koolthing.conf
	@echo "Removed koolthing binaries, systemd unit, and D-Bus configuration."

uninstall-extension:
	./extension/install.sh uninstall

clean:
	rm -rf $(BIN_DIR)
	rm -f *.shell-extension.zip *.zip extension/*.zip extension/*.shell-extension.zip
	@echo "Cleaned build artifacts."
