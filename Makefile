.PHONY: build test bench run clean lint fmt vet client

# 构建
build:
	go build -o bin/server ./cmd/server
	go build -o bin/cli ./cmd/cli
	go build -o bin/client ./cmd/client

# 构建客户端
client:
	go build -o bin/client ./cmd/client

# 运行服务端
run: build
	./bin/server

# 运行客户端
run-client: client
	./bin/client

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

# 测试客户端
test-client: client
	./test_client.sh

# 运行示例
example: build
	./bin/server &
	sleep 2
	./bin/client -server localhost:8400
	./bin/client -server localhost:8400 sessions

# Tidy 依赖
tidy:
	go mod tidy

# 测试覆盖率
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# 编译检查
check: fmt vet test
