.PHONY: help build build-all install test bench race lint fmt vet
.PHONY: clean clean-all run run-cli run-client run-mcp mock tidy cover check
.PHONY: ui-install ui-build ui-dev ui-preview
.PHONY: build-linux build-darwin build-windows
.PHONY: dist dist-linux dist-darwin dist-windows dist-tarball dist-zip
.PHONY: checksums
.PHONY: npm-version npm-packages npm-pack npm-publish-all npm-publish
.PHONY: pypi-version pypi-packages pypi-pack pypi-publish

# Variables
BINARY_NAME=andb
VERSION=$(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
PRE_VERSION=$(if $(filter %-pre,$(VERSION)),$(VERSION),$(VERSION)-pre)
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"
GOBUILD_FLAGS=-trimpath
DIST_DIR=dist
CHECKSUM_FILE=$(DIST_DIR)/checksums.txt
PYTHON ?= python3
NPM ?= npm

# Python venv for PyPI builds (isolated from system Python)
PYPI_VENV := $(CURDIR)/pypi/.venv-build
PYPI_PYTHON := $(PYPI_VENV)/bin/python

# Create venv and install build deps (idempotent)
$(PYPI_VENV)/bin/python:
	@echo "Creating PyPI build venv..."
	python3 -m venv $(PYPI_VENV)
	$(PYPI_VENV)/bin/pip install -q --upgrade "setuptools>=77.0.0" build twine
	@echo "PyPI build deps ready: $(PYPI_VENV)"

# UPX compression (skip for macOS — not supported)
USE_UPX ?= true
ifeq ($(shell which upx 2>/dev/null),)
USE_UPX = false
endif
ifeq ($(USE_UPX),true)
UPX_CMD = upx -9
else
UPX_CMD = @true
endif

# ======================== 帮助 ========================

help:
	@echo "AgentNativeDB Build System"
	@echo ""
	@echo "Build targets:"
	@echo "  build            Build for current platform (with Web UI)"
	@echo "  build-linux      Build for Linux (amd64, arm64)"
	@echo "  build-darwin     Build for macOS (amd64, arm64)"
	@echo "  build-windows    Build for Windows (amd64)"
	@echo "  build-all        Build for all platforms"
	@echo ""
	@echo "Distribution targets:"
	@echo "  dist             Build all distribution packages"
	@echo "  dist-linux       Build Linux packages (tar.gz)"
	@echo "  dist-darwin      Build macOS packages (tar.gz)"
	@echo "  dist-windows     Build Windows packages (zip)"
	@echo "  dist-tarball     Build tarball packages only"
	@echo "  dist-zip         Build zip packages only"
	@echo "  checksums        Generate checksums for all dist files"
	@echo ""
	@echo "NPM targets:"
	@echo "  npm-version      Sync version to npm package"
	@echo "  npm-packages     Build platform-specific npm packages"
	@echo "  npm-pack         Pack npm packages"
	@echo "  npm-publish-all  Publish all npm packages"
	@echo "  npm-publish      Publish main package only"
	@echo ""
	@echo "PyPI targets:"
	@echo "  pypi-version     Sync version to PyPI package"
	@echo "  pypi-packages    Build platform-specific PyPI wheels"
	@echo "  pypi-pack        Pack PyPI wheels"
	@echo "  pypi-publish     Publish PyPI wheels"
	@echo ""
	@echo "UI targets:"
	@echo "  ui-install       Install Web UI dependencies"
	@echo "  ui-build         Build Web UI into ui/dist"
	@echo "  ui-dev           Run Web UI dev server"
	@echo "  ui-preview       Preview built Web UI"
	@echo ""
	@echo "Other targets:"
	@echo "  install          Install via go install"
	@echo "  test             Run tests (with race detector)"
	@echo "  bench            Run benchmarks"
	@echo "  race             Run tests with race detector"
	@echo "  lint             Run linter (fmt + vet)"
	@echo "  fmt              Format code"
	@echo "  vet              Run go vet"
	@echo "  clean            Remove build artifacts"
	@echo "  clean-all        Remove everything including dist"
	@echo "  run              Build and run server"
	@echo "  run-cli          Build and run CLI"
	@echo "  run-client       Build and run client"
	@echo "  run-mcp          Build and run MCP server"
	@echo "  mock             Generate mock data (requires server)"
	@echo "  tidy             Tidy Go modules"
	@echo "  cover            Generate coverage report"
	@echo "  check            Full check (fmt + vet + test)"
	@echo "  help             Show this help"

# ======================== 构建 ========================

# 构建 Web UI
ui-install:
	cd ui && $(NPM) ci

ui-build:
	cd ui && $(NPM) run build

ui-dev:
	cd ui && $(NPM) run dev

ui-preview:
	cd ui && $(NPM) run preview

# 构建单二进制 (包含 Web UI)
build: ui-build
	go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/andb

# 仅构建 Go 二进制（不重建 UI）
build-go:
	go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/andb

# ======================== 平台构建 ========================

build-linux:
	@echo "Building for Linux..."
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/andb
	GOOS=linux GOARCH=arm64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/andb
	@echo "Compressing Linux amd64 binary with UPX..."
	$(UPX_CMD) bin/$(BINARY_NAME)-linux-amd64

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p bin
	GOOS=darwin GOARCH=amd64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/andb
	GOOS=darwin GOARCH=arm64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/andb

build-windows:
	@echo "Building for Windows..."
	@mkdir -p bin
	GOOS=windows GOARCH=amd64 go build $(GOBUILD_FLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe ./cmd/andb
	@echo "Compressing Windows amd64 binary with UPX..."
	$(UPX_CMD) bin/$(BINARY_NAME)-windows-amd64.exe

# 构建所有平台
build-all: build-linux build-darwin build-windows
	@echo ""
	@echo "Build complete! Binaries in bin/"
	@ls -lh bin/

# ======================== 安装/运行 ========================

install:
	go install $(GOBUILD_FLAGS) $(LDFLAGS) ./cmd/andb

run: build
	./bin/$(BINARY_NAME) server

run-cli: build
	./bin/$(BINARY_NAME) cli

run-client: build
	./bin/$(BINARY_NAME) client

run-mcp: build
	./bin/$(BINARY_NAME) server -mode mcp

# ======================== 测试/检查 ========================

test:
	go test -v -race -count=1 ./...

bench:
	go test -bench=. -benchmem ./...

race:
	go test -race -count=1 ./...

lint: fmt vet

fmt:
	go fmt ./...

vet:
	go vet ./...

check: fmt vet test

# 覆盖率
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# 依赖整理
tidy:
	go mod tidy

# Mock 数据生成 (需要先启动服务器)
mock:
	go run ./cmd/mock

# ======================== 清理 ========================

clean:
	rm -rf bin/ data/

clean-all: clean
	rm -rf $(DIST_DIR)
	rm -f npm/*.tgz
	rm -rf pypi/dist pypi/build pypi/*.egg-info $(PYPI_VENV)

# ======================== 分发包 ========================

# tar.gz for Linux / macOS
dist-tarball: build-linux build-darwin
	@echo ""
	@echo "Creating tarball packages..."
	@mkdir -p $(DIST_DIR)
	@for arch in amd64 arm64; do \
		echo "  Packaging $(BINARY_NAME)-linux-$${arch}.tar.gz..."; \
		tar -czf $(DIST_DIR)/$(BINARY_NAME)-linux-$${arch}.tar.gz \
			-C bin $(BINARY_NAME)-linux-$${arch}; \
	done
	@for arch in amd64 arm64; do \
		echo "  Packaging $(BINARY_NAME)-darwin-$${arch}.tar.gz..."; \
		tar -czf $(DIST_DIR)/$(BINARY_NAME)-darwin-$${arch}.tar.gz \
			-C bin $(BINARY_NAME)-darwin-$${arch}; \
	done

# zip for Windows
dist-zip: build-windows
	@echo ""
	@echo "Creating Windows zip packages..."
	@mkdir -p $(DIST_DIR)
	@cd bin && zip -j ../$(DIST_DIR)/$(BINARY_NAME)-windows-amd64.zip $(BINARY_NAME)-windows-amd64.exe

dist-linux: dist-tarball
	@echo "Linux packages complete!"

dist-darwin: dist-tarball
	@echo "macOS packages complete!"

dist-windows: dist-zip
	@echo "Windows packages complete!"

# 生成校验和
checksums:
	@echo "Generating checksums..."
	@cd $(DIST_DIR) && \
	find . -type f \( -name "*.tar.gz" -o -name "*.zip" \) | sort | \
	while read f; do \
		sha256sum "$$f"; \
	done > checksums.txt
	@echo "Checksums written to $(CHECKSUM_FILE)"
	@cat $(CHECKSUM_FILE)

# 构建所有分发包
dist: dist-linux dist-darwin dist-windows checksums
	@echo ""
	@echo "=========================================="
	@echo "All distribution packages built!"
	@echo ""
	@echo "Location: $(DIST_DIR)/"
	@ls -lh $(DIST_DIR)/ 2>/dev/null || true
	@echo ""
	@echo "Checksums: $(CHECKSUM_FILE)"
	@echo "=========================================="

# ======================== NPM ========================

npm-version:
	@echo "Syncing version $(VERSION) to npm packages..."
	@cd npm && $(PYTHON) -c "\
	import json, sys; \
	pkg = json.load(open('package.json')); \
	pkg['version'] = '$(VERSION)'; \
	json.dump(pkg, open('package.json', 'w'), indent=2); \
	print(f'  package.json -> {pkg[\"version\"]}')\
	"
	@for d in npm/packages/*/; do \
		if [ -f "$$d/package.json" ]; then \
			cd "$$d" && $(PYTHON) -c "\
import json; \
pkg = json.load(open('package.json')); \
pkg['version'] = '$(VERSION)'; \
json.dump(pkg, open('package.json', 'w'), indent=2); \
print(f'  $$d -> {pkg[\"version\"]}')\
			" && cd - > /dev/null; \
		fi; \
	done

npm-packages: build-all
	@echo "Copying binaries to platform packages..."
	@mkdir -p npm/packages/andb-installer-linux-x64/bin
	cp bin/$(BINARY_NAME)-linux-amd64 npm/packages/andb-installer-linux-x64/bin/andb
	@mkdir -p npm/packages/andb-installer-linux-arm64/bin
	cp bin/$(BINARY_NAME)-linux-arm64 npm/packages/andb-installer-linux-arm64/bin/andb
	@mkdir -p npm/packages/andb-installer-darwin-x64/bin
	cp bin/$(BINARY_NAME)-darwin-amd64 npm/packages/andb-installer-darwin-x64/bin/andb
	@mkdir -p npm/packages/andb-installer-darwin-arm64/bin
	cp bin/$(BINARY_NAME)-darwin-arm64 npm/packages/andb-installer-darwin-arm64/bin/andb
	@mkdir -p npm/packages/andb-installer-win32-x64/bin
	cp bin/$(BINARY_NAME)-windows-amd64.exe npm/packages/andb-installer-win32-x64/bin/andb.exe
	@echo "Platform packages ready."

npm-pack: npm-version npm-packages
	@echo "Packing platform packages..."
	@set -e; for d in npm/packages/*/; do \
		if [ -f "$$d/package.json" ]; then \
			echo "  Packing $$(basename $$d)..."; \
			cd "$$d" && npm pack && cd - > /dev/null; \
			mv "$$d"/*.tgz npm/ 2>/dev/null || true; \
		fi; \
	done
	@echo "Packing andb-installer package..."
	cd npm && npm pack
	@echo "Done. Tarballs in npm/"

npm-publish-all: npm-version npm-packages
	@echo "Publishing platform packages..."
	@set -e; for d in npm/packages/*/; do \
		if [ -f "$$d/package.json" ]; then \
			echo "  Publishing $$(basename $$d)..."; \
			cd "$$d" && $(NPM) publish --access public && cd - > /dev/null; \
		fi; \
	done
	@echo "Publishing andb-installer package..."
	cd npm && $(NPM) publish --access public
	@echo "Published all packages!"

npm-publish: npm-version npm-packages
	@echo "Publishing andb-installer package..."
	cd npm && $(NPM) publish --access public

# ======================== PyPI ========================

pypi-version: PYTHON := $(PYPI_PYTHON)
pypi-packages: PYTHON := $(PYPI_PYTHON)
pypi-pack: PYTHON := $(PYPI_PYTHON)
pypi-publish: PYTHON := $(PYPI_PYTHON)

pypi-version: $(PYPI_VENV)/bin/python
	@echo "Syncing version $(VERSION) to PyPI package..."
	@$(PYTHON) -c "\
	import re; \
	pyproject = open('pypi/pyproject.toml').read(); \
(pyproject, count) = re.subn(r'version = \"[^\"]+\"', 'version = \"$(VERSION)\"', pyproject, count=1); \
	open('pypi/pyproject.toml', 'w').write(pyproject); \
	print(f'  pyproject.toml -> $(VERSION)')\
	"
	@$(PYTHON) -c "\
	content = open('pypi/src/andb_installer/__init__.py').read(); \
(content, count) = re.subn(r'__version__ = \"[^\"]+\"', '__version__ = \"$(VERSION)\"', content, count=1); \
	open('pypi/src/andb_installer/__init__.py', 'w').write(content); \
	print(f'  __init__.py -> $(VERSION)')\
	"

pypi-packages: build-all $(PYPI_VENV)/bin/python
	@echo "Building PyPI platform wheels..."
	@mkdir -p pypi/src/andb_installer/bin
	cp bin/$(BINARY_NAME)-linux-amd64 pypi/src/andb_installer/bin/andb-linux-amd64
	cp bin/$(BINARY_NAME)-linux-arm64 pypi/src/andb_installer/bin/andb-linux-arm64
	cp bin/$(BINARY_NAME)-darwin-amd64 pypi/src/andb_installer/bin/andb-darwin-amd64
	cp bin/$(BINARY_NAME)-darwin-arm64 pypi/src/andb_installer/bin/andb-darwin-arm64
	cp bin/$(BINARY_NAME)-windows-amd64.exe pypi/src/andb_installer/bin/andb-windows-amd64.exe
	cd pypi && $(PYTHON) -m build --wheel
	@echo "PyPI wheels ready in pypi/dist/"

pypi-pack: pypi-version pypi-packages
	@echo "Done. Wheels in pypi/dist/"

pypi-publish: pypi-pack $(PYPI_VENV)/bin/python
	$(PYTHON) -m twine upload pypi/dist/*.whl
