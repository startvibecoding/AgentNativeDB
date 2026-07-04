package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/startvibecoding/AgentNativeDB/internal/model"
	"github.com/startvibecoding/AgentNativeDB/internal/storage"
	"github.com/startvibecoding/AgentNativeDB/internal/util"
)

// Coordinator 多 Agent 协调器
type Coordinator struct {
	engine  storage.Engine
	sessions *SessionManager
	memory   *MemoryStore
	decision *DecisionRecorder

	mu       sync.RWMutex
	rooms    map[string]*Room // roomID -> Room
}

// NewCoordinator 创建协调器
func NewCoordinator(engine storage.Engine, sessions *SessionManager, memory *MemoryStore, decision *DecisionRecorder) *Coordinator {
	return &Coordinator{
		engine:   engine,
		sessions: sessions,
		memory:   memory,
		decision: decision,
		rooms:    make(map[string]*Room),
	}
}

// CreateRoom 创建协作房间
func (c *Coordinator) CreateRoom(ctx context.Context, name string, creatorID string, opts RoomOptions) (*Room, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	room := &Room{
		ID:        util.NewUUID(),
		Name:      name,
		CreatorID: creatorID,
		Options:   opts,
		Members:   []Member{{AgentID: creatorID, Role: RoleOwner, JoinedAt: time.Now()}},
		State:     RoomActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	c.rooms[room.ID] = room

	// 持久化
	if err := c.saveRoom(ctx, room); err != nil {
		return nil, err
	}

	slog.Info("room created", "room_id", room.ID, "name", name, "creator", creatorID)
	return room, nil
}

// GetRoom 获取房间
func (c *Coordinator) GetRoom(ctx context.Context, roomID string) (*Room, error) {
	c.mu.RLock()
	room, ok := c.rooms[roomID]
	c.mu.RUnlock()

	if ok {
		return room, nil
	}

	// 从存储加载
	key := storage.EncodeKey(storage.PrefixSystem, "room:"+roomID)
	data, err := c.engine.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("room not found: %s", roomID)
	}

	var loaded Room
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.rooms[loaded.ID] = &loaded
	c.mu.Unlock()

	return &loaded, nil
}

// JoinRoom Agent 加入房间
func (c *Coordinator) JoinRoom(ctx context.Context, roomID, agentID string, role Role) error {
	room, err := c.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已在房间中
	for _, m := range room.Members {
		if m.AgentID == agentID {
			return fmt.Errorf("agent %s already in room", agentID)
		}
	}

	room.Members = append(room.Members, Member{
		AgentID:  agentID,
		Role:     role,
		JoinedAt: time.Now(),
	})
	room.UpdatedAt = time.Now()

	c.saveRoom(ctx, room)
	slog.Info("agent joined room", "room_id", roomID, "agent_id", agentID)
	return nil
}

// LeaveRoom Agent 离开房间
func (c *Coordinator) LeaveRoom(ctx context.Context, roomID, agentID string) error {
	room, err := c.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for i, m := range room.Members {
		if m.AgentID == agentID {
			room.Members = append(room.Members[:i], room.Members[i+1:]...)
			room.UpdatedAt = time.Now()
			c.saveRoom(ctx, room)
			slog.Info("agent left room", "room_id", roomID, "agent_id", agentID)
			return nil
		}
	}

	return fmt.Errorf("agent %s not in room", agentID)
}

// SendMessage 发送消息到房间
func (c *Coordinator) SendMessage(ctx context.Context, roomID, fromID string, msgType MessageType, content any) (*Message, error) {
	room, err := c.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}

	// 验证发送者在房间中
	if !room.HasMember(fromID) {
		return nil, fmt.Errorf("agent %s not in room", fromID)
	}

	msg := &Message{
		ID:        util.NewUUID(),
		RoomID:    roomID,
		FromID:    fromID,
		Type:      msgType,
		Content:   content,
		Timestamp: time.Now(),
	}

	// 存储消息
	data, _ := json.Marshal(msg)
	key := storage.EncodeKey(storage.PrefixSystem, "msg:"+msg.ID)
	c.engine.Set(ctx, key, data)

	// 更新房间消息索引
	idxKey := storage.EncodeIndexKey(storage.PrefixSystem, "roommsg:"+roomID, msg.ID)
	c.engine.Set(ctx, idxKey, []byte{1})

	c.mu.Lock()
	room.UpdatedAt = time.Now()
	c.saveRoom(ctx, room)
	c.mu.Unlock()

	return msg, nil
}

// GetMessages 获取房间消息
func (c *Coordinator) GetMessages(ctx context.Context, roomID string, limit int) ([]*Message, error) {
	prefix := storage.EncodeIndexKey(storage.PrefixSystem, "roommsg:"+roomID, "")
	start, end := storage.PrefixRange(prefix)

	iter, err := c.engine.Scan(ctx, start, end, storage.ScanOptions{Limit: limit})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var messages []*Message
	for iter.Next() {
		key, _ := iter.Item()
		msgID := storage.DecodeIndexID(key)

		msgKey := storage.EncodeKey(storage.PrefixSystem, "msg:"+msgID)
		data, err := c.engine.Get(ctx, msgKey)
		if err != nil {
			continue
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}

// ShareMemory 共享记忆到房间（所有成员可见）
func (c *Coordinator) ShareMemory(ctx context.Context, roomID, fromID, content string, importance float32) (*model.MemoryEntry, error) {
	room, err := c.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}

	if !room.HasMember(fromID) {
		return nil, fmt.Errorf("agent %s not in room", fromID)
	}

	// 为每个成员创建记忆
	var lastMem *model.MemoryEntry
	for _, member := range room.Members {
		mem := model.NewMemory(member.AgentID, model.MemoryWorking, content, importance)
		mem.ID = util.NewUUID()
		stored, err := c.memory.Store(ctx, mem)
		if err != nil {
			slog.Error("share memory failed", "agent_id", member.AgentID, "error", err)
			continue
		}
		lastMem = stored
	}

	// 发送系统消息
	c.SendMessage(ctx, roomID, fromID, MsgSharedMemory, map[string]any{
		"content": content,
		"members": len(room.Members),
	})

	return lastMem, nil
}

// ListRooms 列出所有房间
func (c *Coordinator) ListRooms(ctx context.Context) ([]*Room, error) {
	prefix := storage.EncodeKey(storage.PrefixSystem, "room:")
	iter, err := c.engine.PrefixScan(ctx, prefix, storage.ScanOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var rooms []*Room
	for iter.Next() {
		_, val := iter.Item()
		var room Room
		if err := json.Unmarshal(val, &room); err == nil && room.ID != "" {
			rooms = append(rooms, &room)
		}
	}
	return rooms, nil
}

// saveRoom 持久化房间
func (c *Coordinator) saveRoom(ctx context.Context, room *Room) error {
	data, err := json.Marshal(room)
	if err != nil {
		return err
	}
	key := storage.EncodeKey(storage.PrefixSystem, "room:"+room.ID)
	return c.engine.Set(ctx, key, data)
}

// Room 协作房间
type Room struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatorID string    `json:"creator_id"`
	Options   RoomOptions `json:"options"`
	Members   []Member  `json:"members"`
	State     RoomState `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// HasMember 检查 Agent 是否在房间中
func (r *Room) HasMember(agentID string) bool {
	for _, m := range r.Members {
		if m.AgentID == agentID {
			return true
		}
	}
	return false
}

// GetMember 获取成员信息
func (r *Room) GetMember(agentID string) *Member {
	for i := range r.Members {
		if r.Members[i].AgentID == agentID {
			return &r.Members[i]
		}
	}
	return nil
}

// RoomOptions 房间选项
type RoomOptions struct {
	MaxMembers     int  `json:"max_members,omitempty"`
	AllowAnonymous bool `json:"allow_anonymous,omitempty"`
	Persistent     bool `json:"persistent,omitempty"`
}

// RoomState 房间状态
type RoomState string

const (
	RoomActive   RoomState = "active"
	RoomArchived RoomState = "archived"
	RoomClosed   RoomState = "closed"
)

// Member 房间成员
type Member struct {
	AgentID  string    `json:"agent_id"`
	Role     Role      `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// Role 成员角色
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
	RoleViewer Role = "viewer"
)

// Message 房间消息
type Message struct {
	ID        string      `json:"id"`
	RoomID    string      `json:"room_id"`
	FromID    string      `json:"from_id"`
	Type      MessageType `json:"type"`
	Content   any         `json:"content"`
	Timestamp time.Time   `json:"timestamp"`
}

// MessageType 消息类型
type MessageType string

const (
	MsgText         MessageType = "text"
	MsgTask         MessageType = "task"
	MsgResult       MessageType = "result"
	MsgSharedMemory MessageType = "shared_memory"
	MsgDecision     MessageType = "decision"
	MsgSystem       MessageType = "system"
)
