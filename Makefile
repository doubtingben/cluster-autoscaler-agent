GO ?= go
BINDIR ?= bin
EXTERNALGRPCCTL := $(BINDIR)/externalgrpcctl

.PHONY: build externalgrpcctl clean

build: externalgrpcctl

externalgrpcctl: $(EXTERNALGRPCCTL)

$(EXTERNALGRPCCTL):
	mkdir -p $(BINDIR)
	$(GO) build -o $(EXTERNALGRPCCTL) ./cmd/externalgrpcctl

clean:
	rm -f $(EXTERNALGRPCCTL)
