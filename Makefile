GO_BUILD_FLAGS :=
DESKTOP_LDFLAGS :=
TRAY_LDFLAGS :=

# ----------------------------
# Build platform selection
# Prefer VIAM_BUILD_* (set by `viam module build local`)
# ----------------------------
BUILD_OS := $(VIAM_BUILD_OS)
BUILD_ARCH := $(VIAM_BUILD_ARCH)

ifeq ($(BUILD_OS),)
  ifeq ($(OS),Windows_NT)
    BUILD_OS := windows
  else
    BUILD_OS := linux
  endif
endif

ifeq ($(BUILD_ARCH),)
  BUILD_ARCH := amd64
endif

# ----------------------------
# Outputs
# ----------------------------
BIN_DIR := bin

MODULE_BINARY := $(BIN_DIR)/testing-windows-ipc
DESKTOP_HELPER_BINARY := $(BIN_DIR)/desktop-helper
TRAY_HELPER_BINARY := $(BIN_DIR)/tray-helper

ifeq ($(BUILD_OS),windows)
  MODULE_BINARY := $(MODULE_BINARY).exe
  DESKTOP_HELPER_BINARY := $(DESKTOP_HELPER_BINARY).exe
  TRAY_HELPER_BINARY := $(TRAY_HELPER_BINARY).exe

  # Hide console windows for helpers (NOT the module)
  DESKTOP_LDFLAGS := -ldflags "-H=windowsgui"
  TRAY_LDFLAGS := -ldflags "-H=windowsgui"
endif

# ----------------------------
# Source lists (Make-safe)
# ----------------------------
ROOT_GO_SRCS := $(wildcard *.go)
MODULE_SRCS := $(wildcard cmd/module/*.go)
DESKTOP_SRCS := $(wildcard cmd/desktop-helper/*.go)
TRAY_SRCS := $(wildcard cmd/tray-helper/*.go)

# ----------------------------
# Phonies
# ----------------------------
.PHONY: bin lint update test setup clean module all

bin:
	mkdir -p $(BIN_DIR)

# ----------------------------
# Build targets
# ----------------------------
$(MODULE_BINARY): Makefile go.mod $(ROOT_GO_SRCS) $(MODULE_SRCS) | bin
	GOOS=$(BUILD_OS) GOARCH=$(BUILD_ARCH) go build $(GO_BUILD_FLAGS) -o $(MODULE_BINARY) cmd/module/main.go

$(DESKTOP_HELPER_BINARY): Makefile go.mod $(DESKTOP_SRCS) | bin
	@if [ -z "$(DESKTOP_SRCS)" ]; then \
		echo "ERROR: no sources found at cmd/desktop-helper/*.go"; \
		exit 1; \
	fi
	GOOS=$(BUILD_OS) GOARCH=$(BUILD_ARCH) go build $(GO_BUILD_FLAGS) $(DESKTOP_LDFLAGS) -o $(DESKTOP_HELPER_BINARY) cmd/desktop-helper/main.go

$(TRAY_HELPER_BINARY): Makefile go.mod $(TRAY_SRCS) | bin
	@if [ -z "$(TRAY_SRCS)" ]; then \
		echo "ERROR: no sources found at cmd/tray-helper/*.go"; \
		exit 1; \
	fi
	GOOS=$(BUILD_OS) GOARCH=$(BUILD_ARCH) go build $(GO_BUILD_FLAGS) $(TRAY_LDFLAGS) -o $(TRAY_HELPER_BINARY) cmd/tray-helper/main.go

lint:
	gofmt -s -w .

update:
	go get go.viam.com/rdk@latest
	go mod tidy

test:
	go test ./...

setup:
	go mod tidy

# ----------------------------
# Packaging
# ----------------------------
module.tar.gz: meta.json $(MODULE_BINARY) $(DESKTOP_HELPER_BINARY) $(TRAY_HELPER_BINARY)
ifeq ($(BUILD_OS),windows)
	@echo "Skipping strip on Windows"
else
	strip $(MODULE_BINARY) || true
	strip $(DESKTOP_HELPER_BINARY) || true
	strip $(TRAY_HELPER_BINARY) || true
endif
	tar czf $@ meta.json $(MODULE_BINARY) $(DESKTOP_HELPER_BINARY) $(TRAY_HELPER_BINARY)

module: test module.tar.gz
all: test module.tar.gz

clean:
	@rm -f module.tar.gz
	@rm -rf $(BIN_DIR)
