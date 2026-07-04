package index

import (
	"strings"
	"unicode"

	"github.com/go-ego/gse"
)

// seg 全局分词器（懒加载）
var seg gse.Segmenter
var segInited bool

func initSeg() {
	if segInited {
		return
	}
	segInited = true
	// 加载内置中文词典（gse 默认包含）
	seg.LoadDict()
}

// Tokenize 将文本切分为 term 序列，用于 Inverted 索引。
//
// 规则：
//   - ASCII 字母/数字 连续段作为一个 token，转小写；
//   - CJK 文本使用 gse 分词器切分（支持中英文混合）；
//   - 空 token 和标点符号被丢弃；
//   - 结果去重，保持插入顺序。
func Tokenize(text string) []string {
	initSeg()

	// gse 分词，模式：Search 模式（细粒度，适合搜索）
	words := seg.CutSearch(text, true)

	var tokens []string
	seen := make(map[string]struct{}, len(words))

	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}

		// ASCII token 转小写
		if isAllASCII(w) {
			w = strings.ToLower(w)
		}

		// 跳过纯标点
		if isPunctuation(w) {
			continue
		}

		if _, ok := seen[w]; ok {
			continue
		}
		seen[w] = struct{}{}
		tokens = append(tokens, w)
	}

	return tokens
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
