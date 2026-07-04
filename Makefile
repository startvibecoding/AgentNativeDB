.PHONY: build test bench run clean lint fmt vet

# 构建
build:
	go build -o bin/server ./cmd/server
	go build -o bin/cli ./cmd/cli

# 运行服务端
run: build
	./bin/server

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

# Tidy 依赖
tidy:
	go mod tidy

# 测试覆盖率
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# 编译检查
check: fmt vet test
