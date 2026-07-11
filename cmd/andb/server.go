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
	_ "github.com/startvibecoding/AgentNativeDB/internal/storage/badger" // 注册 badger 引擎
	andbraft "github.com/startvibecoding/AgentNativeDB/internal/raft"
	"github.com/startvibecoding/AgentNativeDB/internal/vector"
)

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	cfgPath := fs.String("config", "", "配置文件路径")
	runMode := fs.String("mode", "server", "运行模式: server, mcp")
	// 集群模式相关命令行参数
	clusterEnabled := fs.Bool("cluster", false, "启用集群模式")
	nodeID := fs.String("node-id", "", "集群节点ID")
	raftAddr := fs.String("raft-addr", "", "Raft通信地址 (默认与HTTP服务同地址)")
	bootstrap := fs.Bool("bootstrap", false, "初始化新集群（仅首次启动单节点时使用）")
	_ = fs.String("join", "", "加入已有集群，指定Leader地址")
	fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// 命令行参数覆盖配置文件
	if *clusterEnabled {
		cfg.Cluster.Enabled = true
	}
	if *nodeID != "" {
		cfg.Cluster.NodeID = *nodeID
	}
	if *raftAddr != "" {
		cfg.Cluster.RaftAddr = *raftAddr
	}
	if *bootstrap {
		cfg.Cluster.Bootstrap = true
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

	// 存储：通过注册表创建引擎
	var engine storage.Engine
	if cfg.Cluster.Enabled {
		// 集群模式：使用Raft存储引擎
		if cfg.Cluster.NodeID == "" {
			cfg.Cluster.NodeID = fmt.Sprintf("node-%d", time.Now().UnixNano())
		}
		if cfg.Cluster.RaftAddr == "" {
			cfg.Cluster.RaftAddr = fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		}
		raftAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		peers := cfg.Cluster.Peers
		if peers == nil {
			peers = make(map[string]string)
		}
		if _, ok := peers[cfg.Cluster.NodeID]; !ok {
			peers[cfg.Cluster.NodeID] = raftAddr
		}
		slog.Info("starting in cluster mode",
			"node_id", cfg.Cluster.NodeID,
			"raft_addr", cfg.Cluster.RaftAddr,
			"peers", len(peers),
			"bootstrap", cfg.Cluster.Bootstrap,
		)
		raftEngine, err := andbraft.NewRaftEngine(andbraft.RaftConfig{
			NodeID:    cfg.Cluster.NodeID,
			RaftAddr:  cfg.Cluster.RaftAddr,
			DataDir:   cfg.Storage.DataDir,
			Peers:     peers,
			Bootstrap: cfg.Cluster.Bootstrap,
		})
		if err != nil {
			log.Fatalf("create raft engine: %v", err)
		}
		engine = raftEngine
	} else {
		// 单机模式
		opts := cfg.Storage.StorageOpts()
		engine, err = storage.CreateEngine(opts)
		if err != nil {
			log.Fatalf("create storage engine: %v", err)
		}
	}
	defer engine.Close()
	slog.Info("storage opened", "cluster", cfg.Cluster.Enabled, "data_dir", cfg.Storage.DataDir)

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
