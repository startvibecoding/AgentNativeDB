package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/startvibecoding/AgentNativeDB/api/http"
	"github.com/startvibecoding/AgentNativeDB/api/mcp"
	"github.com/startvibecoding/AgentNativeDB/config"
	"github.com/startvibecoding/AgentNativeDB/internal/agent"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	badgerstore "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
)

func main() {
	cfgPath := flag.String("config", "", "配置文件路径")
	runMode := flag.String("mode", "server", "运行模式: server, mcp")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// 初始化日志
	var logLevel slog.Level
	switch cfg.Log.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	// 打开存储引擎
	engine := badgerstore.New()
	opts := storage.Options{
		DataDir:          cfg.Storage.DataDir,
		SyncWrites:       cfg.Storage.SyncWrites,
		ValueLogFileSize: cfg.Storage.ValueLogFileSize,
		MemTableSize:     cfg.Storage.MemTableSize,
		NumMemTables:     cfg.Storage.NumMemTables,
		CacheSizeMB:      cfg.Storage.CacheSizeMB,
	}

	if err := engine.Open(opts); err != nil {
		log.Fatalf("open storage: %v", err)
	}
	defer engine.Close()

	slog.Info("storage engine opened", "data_dir", cfg.Storage.DataDir)

	// 创建缓存
	cache := storage.NewCache(512)

	// 创建 Agent 组件
	sessionMgr := agent.NewSessionManager(engine, cache)
	memoryStore := agent.NewMemoryStore(engine, cache)
	decisionRecorder := agent.NewDecisionRecorder(engine, cache)

	// 根据模式运行
	if *runMode == "mcp" {
		runMCPMode(engine, sessionMgr, memoryStore, decisionRecorder)
		return
	}

	// 创建 HTTP 路由
	router := apihttp.NewRouter(engine, sessionMgr, memoryStore, decisionRecorder)

	// 创建 HTTP 服务器
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// 启动服务器
	go func() {
		slog.Info("starting server", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}

	slog.Info("server stopped")
}

func runMCPMode(engine storage.Engine, session *agent.SessionManager, memory *agent.MemoryStore, decision *agent.DecisionRecorder) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	srv := mcp.NewMCPServer(engine, session, memory, decision)
	if err := srv.Run(context.Background()); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}

// printJSON 美化打印 JSON（调试用）
func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}
