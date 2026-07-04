package storage

import (
	"testing"
)

func TestEncodeKey(t *testing.T) {
	key := EncodeKey(PrefixSession, "abc-123")
	if len(key) != 1+len("abc-123") {
		t.Fatalf("expected length %d, got %d", 1+len("abc-123"), len(key))
	}
	if key[0] != PrefixSession {
		t.Fatalf("expected prefix 0x%02x, got 0x%02x", PrefixSession, key[0])
	}
	if string(key[1:]) != "abc-123" {
		t.Fatalf("expected 'abc-123', got %q", string(key[1:]))
	}
}

func TestEncodeIndexKey(t *testing.T) {
	key := EncodeIndexKey(PrefixSession, "agent-001", "sess-123")
	if key[0] != PrefixSession {
		t.Fatalf("expected prefix 0x%02x, got 0x%02x", PrefixSession, key[0])
	}

	field := DecodeIndexField(key)
	if field != "agent-001" {
		t.Fatalf("expected field 'agent-001', got %q", field)
	}

	id := DecodeIndexID(key)
	if id != "sess-123" {
		t.Fatalf("expected id 'sess-123', got %q", id)
	}
}

func TestPrefixRange(t *testing.T) {
	start, end := PrefixRange([]byte{PrefixSession})

	if len(start) != 1 || start[0] != PrefixSession {
		t.Fatalf("unexpected start: %x", start)
	}

	if len(end) != 1 || end[0] != PrefixSession+1 {
		t.Fatalf("unexpected end: %x", end)
	}
}

func TestPrefixRange_AllFF(t *testing.T) {
	start, end := PrefixRange([]byte{0xFF})

	if len(start) != 1 || start[0] != 0xFF {
		t.Fatalf("unexpected start: %x", start)
	}

	if end != nil {
		t.Fatalf("expected nil end for all-FF prefix, got %x", end)
	}
}

func TestFloat32BytesRoundTrip(t *testing.T) {
	original := []float32{1.0, 2.5, -3.14, 0.0, 100.0}
	data := Float32sToBytes(original)
	decoded := BytesToFloat32s(data)

	if len(decoded) != len(original) {
		t.Fatalf("length mismatch: %d vs %d", len(decoded), len(original))
	}

	for i, v := range original {
		if decoded[i] != v {
			t.Fatalf("value mismatch at %d: %f vs %f", i, v, decoded[i])
		}
	}
}

func TestFloat32Bytes_Empty(t *testing.T) {
	data := Float32sToBytes(nil)
	if len(data) != 0 {
		t.Fatalf("expected empty bytes, got %d bytes", len(data))
	}

	floats := BytesToFloat32s(nil)
	if len(floats) != 0 {
		t.Fatalf("expected empty floats, got %d floats", len(floats))
	}
}
