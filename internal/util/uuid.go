package util

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

var (
	lastTimestamp uint64
	sequence     uint16
	mu           sync.Mutex
)

// NewUUID 生成 UUID v7（时间有序）
// 格式: [48bit 时间戳毫秒][4bit 版本][12bit 随机序列][2bit 变体][62bit 随机]
func NewUUID() string {
	mu.Lock()
	defer mu.Unlock()

	now := uint64(time.Now().UnixMilli())

	// 同一毫秒内递增序列号
	if now == lastTimestamp {
		sequence++
		if sequence >= 4096 {
			// 序列号溢出，等待下一毫秒
			for now == lastTimestamp {
				now = uint64(time.Now().UnixMilli())
			}
			sequence = 0
		}
	} else {
		sequence = 0
	}
	lastTimestamp = now

	// 生成随机字节
	var randBytes [10]byte
	_, err := rand.Read(randBytes[:])
	if err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}

	// 构造 UUID v7
	// 字节 0-5: 时间戳毫秒（48bit）
	// 字节 6-7: 版本(0111) + 序列号(12bit)
	// 字节 8:   变体(10) + 随机(6bit)
	// 字节 9-15: 随机(56bit)

	var uuid [16]byte

	// 时间戳 48bit
	binary.BigEndian.PutUint32(uuid[0:4], uint32(now>>16))
	binary.BigEndian.PutUint16(uuid[4:6], uint16(now&0xFFFF))

	// 版本 7 + 序列号 12bit
	uuid[6] = byte(0x70 | (sequence >> 8))
	uuid[7] = byte(sequence & 0xFF)

	// 变体 10 + 随机 6bit
	uuid[8] = byte(0x80 | (randBytes[0]&0x3F))

	// 随机 56bit
	copy(uuid[9:], randBytes[1:8])

	return formatUUID(uuid)
}

func formatUUID(uuid [16]byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(uuid[0:4]),
		binary.BigEndian.Uint16(uuid[4:6]),
		binary.BigEndian.Uint16(uuid[6:8]),
		binary.BigEndian.Uint16(uuid[8:10]),
		uint64(binary.BigEndian.Uint16(uuid[10:12]))<<32|
			uint64(binary.BigEndian.Uint32(uuid[12:16])),
	)
}

// IsValidUUID 检查 UUID 格式是否合法
func IsValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHexChar(c) {
				return false
			}
		}
	}
	return true
}

func isHexChar(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
