package config

import (
	"encoding/json"
	"os"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/storage"
)

// Config 数据库配置
type Config struct {
	Server  ServerConfig  `json:"server"`
	Storage StorageConfig `json:"storage"`
	Agent   AgentConfig   `json:"agent"`
	Vector  VectorConfig  `json:"vector"`
	Log     LogConfig     `json:"log"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
}

// StorageConfig 存储配置
type StorageConfig struct {
	Backend     string `json:"backend"`     // 存储引擎名称，如 "badger"
	DataDir     string `json:"data_dir"`
	SyncWrites  bool   `json:"sync_writes"`
	CacheSizeMB int    `json:"cache_size_mb"`

	// Badger 专属配置（仅 Backend=badger 时生效）
	Badger BadgerConfig `json:"badger"`
}

// BadgerConfig BadgerDB 专属配置
type BadgerConfig struct {
	ValueLogFileSize int64 `json:"value_log_file_size"`
	MemTableSize     int64 `json:"mem_table_size"`
	NumMemTables     int   `json:"num_mem_tables"`
}

// StorageOpts 将 StorageConfig 转为 storage.Options，供 storage.CreateEngine 使用。
func (sc StorageConfig) StorageOpts() storage.Options {
	backendOpts := map[string]any{}
	if sc.Badger.ValueLogFileSize > 0 {
		backendOpts["value_log_file_size"] = sc.Badger.ValueLogFileSize
	}
	if sc.Badger.MemTableSize > 0 {
		backendOpts["mem_table_size"] = sc.Badger.MemTableSize
	}
	if sc.Badger.NumMemTables > 0 {
		backendOpts["num_mem_tables"] = sc.Badger.NumMemTables
	}
	return storage.Options{
		Backend:     sc.Backend,
		DataDir:     sc.DataDir,
		SyncWrites:  sc.SyncWrites,
		CacheSizeMB: sc.CacheSizeMB,
		BackendOpts: backendOpts,
	}
}

// AgentConfig Agent 配置
type AgentConfig struct {
	SessionTimeout  time.Duration `json:"session_timeout"`
	MaxMemories     int           `json:"max_memories"`
	ShortTermWindow int           `json:"short_term_window"`
	CleanupInterval time.Duration `json:"cleanup_interval"`
}

// VectorConfig 向量配置
type VectorConfig struct {
	DefaultDimension   int    `json:"default_dimension"`
	DefaultMetric      string `json:"default_metric"`
	HNSWM              int    `json:"hnsw_m"`
	HNSWEfConstruction int    `json:"hnsw_ef_construction"`
	HNSWEfSearch       int    `json:"hnsw_ef_search"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"` // "text" or "json"
}

// Load 从 JSON 文件加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Default(), nil // 返回默认配置
	}
	cfg := Default()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Default 返回默认配置
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:         "0.0.0.0",
			Port:         8400,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Storage: StorageConfig{
			Backend:     storage.BackendBadger,
			DataDir:     "./data",
			SyncWrites:  true,
			CacheSizeMB: 256,
			Badger: BadgerConfig{
				ValueLogFileSize: 64 << 20, // 64MB
				MemTableSize:     16 << 20, // 16MB
				NumMemTables:     3,
			},
		},
		Agent: AgentConfig{
			SessionTimeout:  24 * time.Hour,
			MaxMemories:     10000,
			ShortTermWindow: 50,
			CleanupInterval: 1 * time.Hour,
		},
		Vector: VectorConfig{
			DefaultDimension:   1536,
			DefaultMetric:      "cosine",
			HNSWM:              16,
			HNSWEfConstruction: 200,
			HNSWEfSearch:       64,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
	}
}
