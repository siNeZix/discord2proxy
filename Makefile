BINARY    := discord2proxy-cli
BINARYGUI := discord2proxy-gui
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

.PHONY: all build build-gui clean

all: build build-gui

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILDDIR)/$(BINARY)$(EXT) ./cmd/

build-gui:
	$(GO) build -ldflags "$(LDFLAGS) -H=windowsgui" -o $(BUILDDIR)/$(BINARYGUI)$(EXT) ./cmd/gui/

clean:
	rm -rf $(BUILDDIR)
