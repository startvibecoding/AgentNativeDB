package index

import (
	"bytes"
	"sort"
	"testing"
)

// ========== 编码测试 ==========

func TestEncodeValue_Order(t *testing.T) {
	cases := []struct {
		a, b any
		want int
	}{
		{nil, false, -1},
		{false, true, -1},
		{int64(1), int64(2), -1},
		{int64(-1), int64(1), -1},
		{float64(1.5), float64(2.5), -1},
		{float64(-2.5), float64(-1.5), -1},
		{float64(0), float64(0), 0},
		{"apple", "banana", -1},
		{"a", "ab", -1},
	}
	for _, c := range cases {
		ea := EncodeValue(c.a)
		eb := EncodeValue(c.b)
		got := bytes.Compare(ea, eb)
		if norm(got) != c.want {
			t.Errorf("EncodeValue(%v) vs (%v): got %d want %d\n  a=%x\n  b=%x", c.a, c.b, got, c.want, ea, eb)
		}
	}
}

func norm(n int) int {
	if n < 0 {
		return -1
	}
	if n > 0 {
		return 1
	}
	return 0
}

func TestEncodeValue_SameTypeOrder(t *testing.T) {
	// 同类型内严格递增
	ints := []any{int64(-100), int64(-1), int64(0), int64(1), int64(100)}
	for i := 1; i < len(ints); i++ {
		ea := EncodeValue(ints[i-1])
		eb := EncodeValue(ints[i])
		if bytes.Compare(ea, eb) >= 0 {
			t.Errorf("int order: %v should be < %v", ints[i-1], ints[i])
		}
	}

	floats := []any{float64(-1.5), float64(0), float64(0.5), float64(1.5)}
	for i := 1; i < len(floats); i++ {
		ea := EncodeValue(floats[i-1])
		eb := EncodeValue(floats[i])
		if bytes.Compare(ea, eb) >= 0 {
			t.Errorf("float order: %v should be < %v", floats[i-1], floats[i])
		}
	}

	strs := []any{"", "a", "ab", "b", "ba"}
	for i := 1; i < len(strs); i++ {
		ea := EncodeValue(strs[i-1])
		eb := EncodeValue(strs[i])
		if bytes.Compare(ea, eb) >= 0 {
			t.Errorf("string order: %q should be < %q", strs[i-1], strs[i])
		}
	}
}

func TestEncodeValue_CrossTypeOrder(t *testing.T) {
	// null < bool < number < string
	orders := []struct {
		a, b any
	}{
		{nil, false},
		{nil, int64(0)},
		{nil, "x"},
		{false, int64(0)},
		{true, "x"},
		{int64(0), "x"},
	}
	for _, o := range orders {
		ea := EncodeValue(o.a)
		eb := EncodeValue(o.b)
		if bytes.Compare(ea, eb) >= 0 {
			t.Errorf("cross-type: %v should be < %v", o.a, o.b)
		}
	}
}

func TestEncodeValue_NoZeroByte(t *testing.T) {
	// 编码后不应包含裸 0x00（会破坏 key 分隔逻辑）
	vals := []any{
		nil, false, true,
		int64(0), int64(-1), int64(1),
		float64(0), float64(-1.5), float64(1.5),
		"", "a", "abc", string([]byte{0, 1, 2}),
	}
	for _, v := range vals {
		enc := EncodeValue(v)
		for i := 1; i < len(enc); i++ {
			if enc[i] == 0x00 {
				if i+1 < len(enc) && enc[i+1] == 0xFF {
					i++ // skip escape sequence 0x00 0xFF
					continue
				}
				t.Errorf("EncodeValue(%v) contains bare 0x00 at byte %d: %x", v, i, enc)
			}
		}
	}
}

func TestEncodeValue_BoolOrder(t *testing.T) {
	eFalse := EncodeValue(false)
	eTrue := EncodeValue(true)
	if bytes.Compare(eFalse, eTrue) >= 0 {
		t.Error("false should be < true")
	}
}

func TestEncodeValue_LargeIntegers(t *testing.T) {
	// 大整数也能保持顺序
	big := []int64{-999999999, -1, 0, 1, 999999999}
	for i := 1; i < len(big); i++ {
		ea := EncodeValue(big[i-1])
		eb := EncodeValue(big[i])
		if bytes.Compare(ea, eb) >= 0 {
			t.Errorf("big int order: %d should be < %d", big[i-1], big[i])
		}
	}
}

func TestEncodeValue_StringNullEscape(t *testing.T) {
	// 含 0x00 的字符串应被转义
	s := "hello\x00world"
	enc := EncodeValue(s)
	// tag(1) + "hello"(5) + 0x00 0xFF(2) + "world"(5) = 13
	if len(enc) != 13 {
		t.Errorf("expected length 13, got %d: %x", len(enc), enc)
	}
	// 确认没有裸 0x00（0x00 后面必须跟 0xFF）
	for i := 1; i < len(enc); i++ {
		if enc[i] == 0x00 {
			if i+1 < len(enc) && enc[i+1] == 0xFF {
				i++ // skip escape sequence
				continue
			}
			t.Errorf("bare 0x00 at byte %d", i)
		}
	}
}

// ========== 分词测试 ==========

func TestTokenize_Basic(t *testing.T) {
	got := Tokenize("Hello, World! Hello again.")
	want := []string{"hello", "world", "again"}
	if !equalSlice(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestTokenize_CJK(t *testing.T) {
	got := Tokenize("北京天气很好")
	has := map[string]bool{}
	for _, g := range got {
		has[g] = true
	}
	if !hasWord(got, "北京") && !hasWord(got, "天气") && len(got) < 2 {
		t.Errorf("expected meaningful tokens, got %v", got)
	}
}

func TestTokenize_Mixed(t *testing.T) {
	got := Tokenize("Go语言是最好的编程语言")
	t.Logf("mixed tokens: %v", got)
	if len(got) < 3 {
		t.Errorf("expected >= 3 tokens, got %d: %v", len(got), got)
	}
}

func TestTokenize_EnglishPhrase(t *testing.T) {
	got := Tokenize("hello world foo bar")
	want := []string{"hello", "world", "foo", "bar"}
	if !equalSlice(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestTokenize_Dedup(t *testing.T) {
	got := Tokenize("hello hello world world")
	seen := map[string]bool{}
	for _, tok := range got {
		if seen[tok] {
			t.Errorf("duplicate token %q", tok)
		}
		seen[tok] = true
	}
}

func TestTokenize_CJK_Bigram(t *testing.T) {
	got := Tokenize("我喜欢学习编程")
	sort.Strings(got)
	t.Logf("programming tokens: %v", got)
	if len(got) < 2 {
		t.Errorf("expected >= 2 tokens, got %v", got)
	}
}

func TestTokenize_EmptyString(t *testing.T) {
	got := Tokenize("")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestTokenize_PunctuationOnly(t *testing.T) {
	got := Tokenize("!!!???...")
	if len(got) != 0 {
		t.Errorf("expected empty for punctuation, got %v", got)
	}
}

func TestTokenize_CaseInsensitive(t *testing.T) {
	got := Tokenize("Hello HELLO hello")
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("expected [hello], got %v", got)
	}
}

func TestTokenize_ChineseSentences(t *testing.T) {
	cases := []struct {
		name  string
		input string
		check func(t *testing.T, tokens []string)
	}{
		{
			name:  "技术文本",
			input: "数据库系统支持向量检索和全文搜索",
			check: func(t *testing.T, tokens []string) {
				if len(tokens) < 3 {
					t.Errorf("expected >= 3 tokens, got %v", tokens)
				}
			},
		},
		{
			name:  "混合标点",
			input: "你好，世界！Hello, World!",
			check: func(t *testing.T, tokens []string) {
				has := hasWord(tokens, "你好") || hasWord(tokens, "世界")
				if !has && len(tokens) < 2 {
					t.Errorf("expected CJK tokens, got %v", tokens)
				}
			},
		},
		{
			name:  "数字混合",
			input: "版本2.0支持100个并发",
			check: func(t *testing.T, tokens []string) {
				if len(tokens) < 2 {
					t.Errorf("expected >= 2 tokens, got %v", tokens)
				}
			},
		},
		{
			name:  "日文假名",
			input: "日本語テスト",
			check: func(t *testing.T, tokens []string) {
				if len(tokens) < 1 {
					t.Errorf("expected >= 1 token, got %v", tokens)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Tokenize(tc.input)
			t.Logf("tokens: %v", got)
			tc.check(t, got)
		})
	}
}

func TestTokenize_EnglishEdgeCases(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"single word", "hello", []string{"hello"}},
		{"numbers", "123 456", []string{"123", "456"}},
		{"alphanumeric", "test123", []string{"test123"}},
		{"mixed case", "GoLang", []string{"golang"}},
		{"empty words", "  hello   world  ", []string{"hello", "world"}},
		{"special chars", "hello@world#test", []string{"hello", "world", "test"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Tokenize(tc.input)
			if !equalSlice(got, tc.want) {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func hasWord(tokens []string, word string) bool {
	for _, t := range tokens {
		if t == word {
			return true
		}
	}
	return false
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
