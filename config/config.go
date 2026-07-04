package config

import (
	"encoding/json"
	"os"
	"time"
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
	DataDir          string `json:"data_dir"`
	ValueLogFileSize int64  `json:"value_log_file_size"`
	MemTableSize     int64  `json:"mem_table_size"`
	NumMemTables     int    `json:"num_mem_tables"`
	SyncWrites       bool   `json:"sync_writes"`
	CacheSizeMB      int    `json:"cache_size_mb"`
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
			DataDir:          "./data",
			ValueLogFileSize: 64 << 20, // 64MB
			MemTableSize:     16 << 20, // 16MB
			NumMemTables:     3,
			SyncWrites:       true,
			CacheSizeMB:      256,
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

// StorageOptions 转换为 storage.Options
func (c *Config) StorageOptions() struct {
	DataDir          string
	SyncWrites       bool
	ValueLogFileSize int64
	MemTableSize     int64
	NumMemTables     int
	CacheSizeMB      int
} {
	return struct {
		DataDir          string
		SyncWrites       bool
		ValueLogFileSize int64
		MemTableSize     int64
		NumMemTables     int
		CacheSizeMB      int
	}{
		DataDir:          c.Storage.DataDir,
		SyncWrites:       c.Storage.SyncWrites,
		ValueLogFileSize: c.Storage.ValueLogFileSize,
		MemTableSize:     c.Storage.MemTableSize,
		NumMemTables:     c.Storage.NumMemTables,
		CacheSizeMB:      c.Storage.CacheSizeMB,
	}
}
