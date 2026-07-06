package index

import (
	"strings"
	"unicode"
)

// Tokenize 将文本切分为 term 序列，用于 Inverted 索引。
//
// 规则：
//   - ASCII 字母/数字 连续段作为一个 token，转小写；
//   - CJK 文本使用搜索分词：bigram 和单字同时入索引；
//   - 空 token 和标点符号被丢弃；
//   - 结果去重，保持插入顺序。
func Tokenize(text string) []string {
	var tokens []string
	seen := make(map[string]struct{})

	emit := func(word string) {
		word = strings.TrimSpace(word)
		if word == "" || isPunctuation(word) {
			return
		}
		if isAllASCII(word) {
			word = strings.ToLower(word)
		}
		if _, ok := seen[word]; ok {
			return
		}
		seen[word] = struct{}{}
		tokens = append(tokens, word)
	}

	var ascii strings.Builder
	var cjk []rune
	flushASCII := func() {
		if ascii.Len() == 0 {
			return
		}
		emit(ascii.String())
		ascii.Reset()
	}
	flushCJK := func() {
		if len(cjk) == 0 {
			return
		}
		tokenizeCJK(cjk, emit)
		cjk = cjk[:0]
	}

	for _, r := range text {
		switch {
		case isASCIIWord(r):
			flushCJK()
			ascii.WriteRune(unicode.ToLower(r))
		case isCJKWord(r):
			flushASCII()
			cjk = append(cjk, r)
		default:
			flushASCII()
			flushCJK()
		}
	}
	flushASCII()
	flushCJK()

	return tokens
}

// tokenizeCJK 生成面向搜索的 CJK term。
//
// bigram 支持常见中文词检索，单字保证短查询和跨词边界仍可命中。
func tokenizeCJK(runes []rune, emit func(string)) {
	n := len(runes)
	if n == 0 {
		return
	}
	for i := 0; i+1 < n; i++ {
		emit(string(runes[i : i+2]))
	}
	for _, r := range runes {
		emit(string(r))
	}
}

// isASCIIWord 判断是否属于 ASCII 单词字符。
func isASCIIWord(r rune) bool {
	return r <= 127 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
}

// isCJKWord 判断是否属于需要按搜索分词处理的非 ASCII 文字。
func isCJKWord(r rune) bool {
	if r <= 127 {
		return false
	}
	switch {
	case unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul):
		return true
	case unicode.IsLetter(r) || unicode.IsDigit(r):
		return true
	default:
		return false
	}
}

// isAllASCII 检查字符串是否全是 ASCII 字符
func isAllASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// isPunctuation 检查字符串是否全是标点符号
func isPunctuation(s string) bool {
	for _, r := range s {
		if !unicode.IsPunct(r) && !unicode.IsSymbol(r) && r != ' ' {
			return false
		}
	}
	return true
}
