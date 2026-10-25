package main

import (
	"runtime"
	"strings"
)

// 文件换行风格。编辑区内部永远只存 '\n'：引擎的 textarea value 口径就是
// LF（解析/SetValue/粘贴都会把 CRLF 归一，见 engine 的 normalizeNewlines），
// 换行风格属于文件层，只在读写边界转换。
const (
	eolCRLF = "\r\n"
	eolLF   = "\n"
	eolCR   = "\r"
)

// defaultEOL 是新建文档的换行风格：Windows 用 CRLF（与系统记事本一致），
// 其它平台用 LF。
func defaultEOL() string {
	if runtime.GOOS == "windows" {
		return eolCRLF
	}
	return eolLF
}

// detectEOL 从文件内容推断换行风格：取出现次数最多的那一种，平票取先出现
// 的；一个换行都没有时退回 defaultEOL。混排文件（少见）按多数派统一，保存
// 后变成单一风格——这与主流编辑器的「检测并保留」口径一致。
func detectEOL(s string) string {
	var crlf, lf, cr int
	first := ""
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\r':
			if i+1 < len(s) && s[i+1] == '\n' {
				crlf++
				i++
				if first == "" {
					first = eolCRLF
				}
			} else {
				cr++
				if first == "" {
					first = eolCR
				}
			}
		case '\n':
			lf++
			if first == "" {
				first = eolLF
			}
		}
	}
	best, bestN := first, crlf
	if lf > bestN {
		best, bestN = eolLF, lf
	}
	if cr > bestN {
		best = eolCR
	}
	if best == "" {
		return defaultEOL()
	}
	return best
}

// withEOL 把内部统一的 LF 文本按 eol 写回文件。写之前再折一次换行，保证
// 即使调用方塞进了 '\r'（理论上不会：SetValue/粘贴/解析都已归一），落盘的
// 也是单一风格。
func withEOL(text, eol string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, eolCRLF, eolLF), eolCR, eolLF)
	if eol == "" || eol == eolLF {
		return text
	}
	return strings.ReplaceAll(text, eolLF, eol)
}
