BINARY    := discord-szx
BINARYGUI := discord-szx-gui
BUILDDIR  := build

ifeq ($(OS),Windows_NT)
EXT := .exe
else
EXT :=
endif

GO      := go
LDFLAGS := -s -w

.PHONY: all build build-gui clean

all: build build-gui

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILDDIR)/$(BINARY)$(EXT) ./cmd/

build-gui:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILDDIR)/$(BINARYGUI)$(EXT) ./cmd/gui/

clean:
	rm -rf $(BUILDDIR)
