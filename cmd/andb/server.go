package main

import (
	"context"
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
	"github.com/startvibecoding/AgentNativeDB/internal/query/sql"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	badgerstore "github.com/startvibecoding/AgentNativeDB/internal/storage/badger"
	"github.com/startvibecoding/AgentNativeDB/internal/vector"
)

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	cfgPath := fs.String("config", "", "配置文件路径")
	runMode := fs.String("mode", "server", "运行模式: server, mcp")
	fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// 日志
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

	// 存储
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
	slog.Info("storage opened", "data_dir", cfg.Storage.DataDir)

	cacheEntries := cfg.Storage.CacheSizeMB * 512
	if cacheEntries <= 0 {
		cacheEntries = 512
	}
	cache := storage.NewCache(cacheEntries)

	sessionMgr := agent.NewSessionManager(engine, cache)
	memoryStore := agent.NewMemoryStore(engine, cache)
	decisionRecorder := agent.NewDecisionRecorder(engine, cache)
	vectorStore := vector.NewVectorStore(engine)

	executor := sql.NewExecutor(engine)
	if err := executor.Init(context.Background()); err != nil {
		log.Fatalf("init executor: %v", err)
	}

	if *runMode == "mcp" {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
		srv := mcp.NewMCPServer(engine, sessionMgr, memoryStore, decisionRecorder, executor)
		if err := srv.Run(context.Background()); err != nil {
			log.Fatalf("mcp server: %v", err)
		}
		return
	}

	router := apihttp.NewRouter(engine, sessionMgr, memoryStore, decisionRecorder, executor, vectorStore)

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		slog.Info("starting server", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

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
