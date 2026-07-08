.PHONY: build test bench run clean lint fmt vet

# 构建 Web UI
ui-build:
	cd ui && npm run build

# 构建单二进制 (包含 Web UI)
build: ui-build
	go build -o bin/andb ./cmd/andb

# 运行服务端
run: build
	./bin/andb server

# 运行 CLI
run-cli: build
	./bin/andb cli

# 运行客户端
run-client: build
	./bin/andb client

# 测试
test:
	go test -v -race -count=1 ./...

# 基准测试
bench:
	go test -bench=. -benchmem ./...

# 竞态检测测试
race:
	go test -race -count=1 ./...

# 代码检查
lint: fmt vet

fmt:
	go fmt ./...

vet:
	go vet ./...

# 清理
clean:
	rm -rf bin/ data/

# Mock 数据生成 (需要先启动服务器)
mock:
	go run ./cmd/mock

# Tidy 依赖
tidy:
	go mod tidy

# 测试覆盖率
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# 编译检查
check: fmt vet test
