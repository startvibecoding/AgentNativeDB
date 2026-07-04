// Package index \u5b9e\u73b0 SQL \u5c42\u7684\u4e8c\u7ea7\u7d22\u5f15\uff1aHash / BTree / Inverted\u3002
//
// \u5b58\u50a8\u5e03\u5c40\uff08\u5747\u5728 PrefixIndex \u547d\u540d\u7a7a\u95f4\u4e0b\uff09\uff1a
//
//	Hash / BTree \u7d22\u5f15\u9879\uff1a
//	  [0x11] "h:"<indexName> 0x00 <encodedValue> 0x00 <rowID>   -> {}
//	  [0x11] "b:"<indexName> 0x00 <encodedValue> 0x00 <rowID>   -> {}
//
//	Inverted \u7d22\u5f15\u9879\uff1a
//	  [0x11] "i:"<indexName> 0x00 <term> 0x00 <rowID>            -> {}
//
//	\u7d22\u5f15\u5143\u4fe1\u606f\u4fdd\u5b58\u5728 PrefixSystem \u4e0b\uff1a
//	  [0xFF] "index:"<indexName>                                  -> IndexMeta JSON
package index

import (
	"encoding/binary"
	"math"
)

// Value tag \u5b57\u8282 \u2014\u2014 \u4fdd\u6301\u8de8\u7c7b\u578b\u5e8f\u6027\u3002
const (
	tagNull   byte = 0x00
	tagFalse  byte = 0x10
	tagTrue   byte = 0x11
	tagNumber byte = 0x20
	tagString byte = 0x30
)

// EncodeValue \u5c06\u4efb\u610f\u503c\u7f16\u7801\u4e3a\u5b57\u5178\u5e8f\u53ef\u6bd4\u8f83\u7684\u5b57\u8282\u4e32\uff08\u5185\u90e8\u4e0d\u542b 0x00\uff09\u3002
func EncodeValue(v any) []byte {
	switch x := v.(type) {
	case nil:
		return []byte{tagNull}
	case bool:
		if x {
			return []byte{tagTrue}
		}
		return []byte{tagFalse}
	case int:
		return encodeNumber(float64(x))
	case int32:
		return encodeNumber(float64(x))
	case int64:
		return encodeNumber(float64(x))
	case float32:
		return encodeNumber(float64(x))
	case float64:
		return encodeNumber(x)
	case string:
		return encodeString(x)
	case []byte:
		return encodeString(string(x))
	}
	return encodeString("")
}

// encodeNumber \u5c06 float64 \u7f16\u7801\u4e3a\u5b57\u5178\u5e8f\u53ef\u6bd4\u8f83\u7684 9 \u5b57\u8282\u3002
func encodeNumber(f float64) []byte {
	bits := math.Float64bits(f)
	if f >= 0 {
		bits ^= 0x8000000000000000
	} else {
		bits = ^bits
	}
	var raw [8]byte
	binary.BigEndian.PutUint32(raw[0:4], uint32(bits>>32))
	binary.BigEndian.PutUint32(raw[4:8], uint32(bits))

	out := []byte{tagNumber}
	for _, b := range raw {
		if b == 0x00 {
			out = append(out, 0x00, 0xFF)
		} else {
			out = append(out, b)
		}
	}
	return out
}

// encodeString \u4f7f\u7528 tag+\u8f6c\u4e49\u540e\u7684\u5b57\u8282\uff08\u5c06 0x00 \u8f6c\u4e49\u4e3a 0x00 0xFF\uff09\uff0c
// \u4ee5\u4fbf\u4e0e\u5916\u5c42\u7528 0x00 \u4f5c\u4e3a\u5b57\u6bb5\u5206\u9694\u7b26\u517c\u5bb9\u3002
func encodeString(s string) []byte {
	buf := make([]byte, 0, len(s)+2)
	buf = append(buf, tagString)
	for i := 0; i < len(s); i++ {
		if s[i] == 0x00 {
			buf = append(buf, 0x00, 0xFF)
		} else {
			buf = append(buf, s[i])
		}
	}
	return buf
}
