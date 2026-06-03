BINARY    := discord2proxy-cli
BINARYGUI := discord2proxy-gui
BINARYSETUP := discord2proxy-setup
BINARYSETUPUPX := discord2proxy-setup-upx
BUILDDIR  := build

ifeq ($(OS),Windows_NT)
EXT := .exe
UPX := $(shell where upx 2>nul)
VERSION := $(shell git describe --tags --always --dirty 2>nul)
else
EXT :=
UPX := $(shell which upx 2>/dev/null)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null)
endif

GO      := go
ifeq ($(strip $(VERSION)),)
VERSION := dev
endif
VERSIONPKG := discord-szx/internal/config
VERSION_CLEAN := $(patsubst v%,%,$(VERSION))
LDFLAGS    := -s -w -X $(VERSIONPKG).Version=$(VERSION_CLEAN)

.PHONY: all build build-gui build-setup build-setup-upx clean

all: build build-gui build-setup build-setup-upx

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILDDIR)/$(BINARY)$(EXT) ./cmd/

build-gui:
	$(GO) build -ldflags "$(LDFLAGS) -H=windowsgui" -o $(BUILDDIR)/$(BINARYGUI)$(EXT) ./cmd/gui/

build-setup:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILDDIR)/$(BINARYSETUP)$(EXT) ./cmd/setup/

build-setup-upx:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILDDIR)/$(BINARYSETUPUPX)$(EXT) ./cmd/setup/
ifneq ($(UPX),)
	upx --best --lzma $(BUILDDIR)/$(BINARYSETUPUPX)$(EXT)
else
	@echo "UPX not found in PATH, skipping compression"
endif

clean:
ifeq ($(OS),Windows_NT)
	@if exist $(BUILDDIR) rmdir /s /q $(BUILDDIR)
else
	rm -rf $(BUILDDIR)
endif
