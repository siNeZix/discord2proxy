BINARY  := discord-szx
BUILDDIR := build

GO      := go
LDFLAGS := -s -w

.PHONY: build clean

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILDDIR)/$(BINARY)$(EXT) ./cmd/

clean:
	rm -rf $(BUILDDIR)
