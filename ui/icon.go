// 本文件是图标表：命令的图标名 → 字形；包文档见 doc.go。
package ui

import (
	"unicode/utf8"

	"github.com/go-comm/ghromex/engine"
)

// iconFontFamily 是图标字体。
//
// Segoe MDL2 Assets 是 Windows 10/11 自带的单色图标字体（segmdl2.ttf），
// 按 16px 网格设计，颜色跟随 CSS 的 color——悬停、禁用、换主题都不需要
// 额外图片资源，也不用管缩放。第二项写文件名：后端的 font-family 解析在
// 家族名不认识时会按「字体文件名」再试一次（家族名本身没有登记）。
const iconFontFamily = `"Segoe MDL2 Assets", segmdl2, "Microsoft YaHei"`

// iconGlyphs 是内置图标名 → 码位（Segoe MDL2 Assets 的字符表）。
//
// 只收组件库常用的命令。码位选的是 Windows 10 起就有的字形，老系统也
// 不会缺字；应用还可以给 WithIcon 直接传字符来自定义。
var iconGlyphs = map[string]string{
	"new":      "\uE710", // Add
	"open":     "\uE8E5", // OpenFile
	"save":     "\uE74E", // Save
	"copy":     "\uE8C8", // Copy
	"cut":      "\uE8C6", // Cut
	"paste":    "\uE77F", // Paste
	"undo":     "\uE7A7", // Undo
	"redo":     "\uE7A6", // Redo
	"search":   "\uE721", // Search
	"replace":  "\uE8AB", // Switch（双向箭头，Windows 上用它表示替换）
	"settings": "\uE713", // Settings
	"info":     "\uE946", // Info
	"delete":   "\uE74D", // Delete
	"refresh":  "\uE72C", // Refresh
	"print":    "\uE749", // Print
	"sort":     "\uE8CB", // Sort（上下箭头）
	"download": "\uE896", // Download
	"upload":   "\uE898", // Upload
	"home":     "\uE80F", // Home
	"back":     "\uE72B", // Back
	"forward":  "\uE72A", // Forward
	"help":     "\uE897", // Help
	"play":     "\uE768", // Play
	"pause":    "\uE769", // Pause
	"document": "\uE8A5", // Document
}

// iconGlyph 把 Icon 字段解析成要显示的字形。
//
// 未登记的短字符串按「你自己给的字形」处理（可以写 "＋" 或别的图标字体
// 里的字符）；长字符串返回空——避免把拼错的图标名（"savee"）当成文字
// 画在按钮上，那比没有图标更难受。
func iconGlyph(name string) string {
	if g, ok := iconGlyphs[name]; ok {
		return g
	}
	if utf8.RuneCountInString(name) <= 2 {
		return name
	}
	return ""
}

// newIcon 生成图标元素；没有可用字形时返回 nil（调用方按「无图标」处理）。
func newIcon(doc engine.HTMLDocument, name string) engine.HTMLElement {
	g := iconGlyph(name)
	if g == "" {
		return nil
	}
	return newEl(doc, "span", "ui-icon", g)
}
