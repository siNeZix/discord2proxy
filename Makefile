BINARY    := discord2proxy-cli
BINARYGUI := discord2proxy-gui
BINARYSETUP := discord2proxy-setup
BUILDDIR  := build

ifeq ($(OS),Windows_NT)
EXT := .exe
else
EXT :=
endif

GO      := go
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
ifeq ($(strip $(VERSION)),)
VERSION := dev
endif
VERSIONPKG := discord-szx/internal/config
LDFLAGS    := -s -w -X $(VERSIONPKG).Version=$(VERSION)

.PHONY: all build build-gui build-setup clean

all: build build-gui build-setup

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILDDIR)/$(BINARY)$(EXT) ./cmd/

build-gui:
	$(GO) build -ldflags "$(LDFLAGS) -H=windowsgui" -o $(BUILDDIR)/$(BINARYGUI)$(EXT) ./cmd/gui/

build-setup:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILDDIR)/$(BINARYSETUP)$(EXT) ./cmd/setup/

clean:
	rm -rf $(BUILDDIR)
