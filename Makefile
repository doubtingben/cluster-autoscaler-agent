GO ?= go
BINDIR ?= bin
EXTERNALGRPCCTL := $(BINDIR)/externalgrpcctl
UNSCHEDULABLEPODSWATCH := $(BINDIR)/unschedulablepodswatch

.PHONY: build externalgrpcctl unschedulablepodswatch clean

build: externalgrpcctl unschedulablepodswatch

externalgrpcctl: $(EXTERNALGRPCCTL)
unschedulablepodswatch: $(UNSCHEDULABLEPODSWATCH)

$(EXTERNALGRPCCTL):
	mkdir -p $(BINDIR)
	$(GO) build -o $(EXTERNALGRPCCTL) ./cmd/externalgrpcctl

$(UNSCHEDULABLEPODSWATCH):
	mkdir -p $(BINDIR)
	$(GO) build -o $(UNSCHEDULABLEPODSWATCH) ./cmd/unschedulablepodswatch

clean:
	rm -f $(EXTERNALGRPCCTL) $(UNSCHEDULABLEPODSWATCH)
