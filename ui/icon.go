// 本文件是图标表：命令的图标名 → 字形；包文档见 doc.go。
package ui

import (
	"sort"
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
// 名称按用途分组；除少量别名（arrow-*、open-file、trash…）外，一个名字
// 对应一个常用命令。码位取自微软公开的 Segoe MDL2 Assets 图标表，都是
// 单字符字形，颜色跟随文字；应用也可以给 WithIcon 直接传字符来自定义。
//
// 参考：https://learn.microsoft.com/windows/apps/design/style/segoe-ui-symbol-font
var iconGlyphs = map[string]string{
	// 文件与编辑
	"new":         "\uE710", // Add
	"open":        "\uE8E5", // OpenFile
	"open-file":   "\uE8E5", // OpenFile（别名）
	"save":        "\uE74E", // Save
	"save-as":     "\uE792", // SaveAs
	"copy":        "\uE8C8", // Copy
	"cut":         "\uE8C6", // Cut
	"paste":       "\uE77F", // Paste
	"undo":        "\uE7A7", // Undo
	"redo":        "\uE7A6", // Redo
	"delete":      "\uE74D", // Delete
	"trash":       "\uE74D", // Delete（别名）
	"search":      "\uE721", // Search
	"replace":     "\uE8AB", // Switch（双向箭头，Windows 上用它表示替换）
	"print":       "\uE749", // Print
	"printer":     "\uE749", // Print（别名）
	"refresh":     "\uE72C", // Refresh
	"reload":      "\uE72C", // Refresh（别名）
	"document":    "\uE8A5", // Document
	"page":        "\uE7C3", // Page
	"folder":      "\uE8B7", // Folder
	"folder-open": "\uE838", // FolderOpen
	"new-folder":  "\uE8F4", // NewFolder
	"code":        "\uE943", // Code
	"edit":        "\uE70F", // Edit
	"rename":      "\uE8AC", // Rename
	"select-all":  "\uE8B3", // SelectAll

	// 方向与导航
	"up":            "\uE74A", // Up
	"down":          "\uE74B", // Down
	"left":          "\uE72B", // Back（左箭头）
	"right":         "\uE72A", // Forward（右箭头）
	"arrow-up":      "\uE74A", // Up（别名）
	"arrow-down":    "\uE74B", // Down（别名）
	"arrow-left":    "\uE72B", // Back（别名）
	"arrow-right":   "\uE72A", // Forward（别名）
	"back":          "\uE72B", // Back
	"forward":       "\uE72A", // Forward
	"home":          "\uE80F", // Home
	"chevron-up":    "\uE70E", // ChevronUp
	"chevron-down":  "\uE70D", // ChevronDown
	"chevron-left":  "\uE76B", // ChevronLeft
	"chevron-right": "\uE76C", // ChevronRight
	"page-left":     "\uE760", // PageLeft
	"page-right":    "\uE761", // PageRight
	"go-to-start":   "\uE8FC", // GoToStart
	"menu":          "\uE700", // GlobalNavigationButton
	"history":       "\uE81C", // History
	"recent":        "\uE823", // Recent
	"location":      "\uE81D", // Location
	"map-pin":       "\uE707", // MapPin
	"map-layers":    "\uE81E", // MapLayers

	// 对齐与排版
	"align-left":    "\uE8E4", // AlignLeft
	"align-center":  "\uE8E3", // AlignCenter
	"align-right":   "\uE8E2", // AlignRight
	"bulleted-list": "\uE8FD", // BulletedList
	"list":          "\uEA37", // List
	"check-list":    "\uE9D5", // CheckList
	"bold":          "\uE8DD", // Bold
	"italic":        "\uE8DB", // Italic
	"underline":     "\uE8DC", // Underline
	"font-increase": "\uE8E8", // FontIncrease
	"font-decrease": "\uE8E7", // FontDecrease
	"font-size":     "\uE8E9", // FontSize
	"font-color":    "\uE8D3", // FontColor
	"highlight":     "\uE7E6", // Highlight

	// 播放与媒体
	"play":           "\uE768", // Play
	"pause":          "\uE769", // Pause
	"stop":           "\uE71A", // Stop
	"record":         "\uE7C8", // Record
	"previous":       "\uE892", // Previous
	"next":           "\uE893", // Next
	"rewind":         "\uEB9E", // Rewind
	"fast-forward":   "\uEB9D", // FastForward
	"shuffle":        "\uE8B1", // Shuffle
	"repeat":         "\uE8EE", // RepeatAll
	"repeat-one":     "\uE8ED", // RepeatOne
	"volume":         "\uE767", // Volume
	"volume-0":       "\uE992", // Volume0
	"volume-1":       "\uE993", // Volume1
	"volume-2":       "\uE994", // Volume2
	"volume-3":       "\uE995", // Volume3
	"mute":           "\uE74F", // Mute
	"speakers":       "\uE7F5", // Speakers
	"headphone":      "\uE7F6", // Headphone
	"fullscreen":     "\uE740", // FullScreen
	"back-to-window": "\uE73F", // BackToWindow
	"movies":         "\uE8B2", // Movies
	"video":          "\uE714", // Video
	"streaming":      "\uE93E", // Streaming
	"slideshow":      "\uE786", // Slideshow
	"projector":      "\uE95D", // Projector

	// 视图与窗口
	"zoom":               "\uE71E", // Zoom
	"zoom-in":            "\uE8A3", // ZoomIn
	"zoom-out":           "\uE71F", // ZoomOut
	"fit-page":           "\uE9A6", // FitPage
	"view":               "\uE890", // View
	"view-all":           "\uE8A9", // ViewAll
	"preview":            "\uE8FF", // Preview
	"dock-left":          "\uE90C", // DockLeft
	"dock-right":         "\uE90D", // DockRight
	"dock-bottom":        "\uE90E", // DockBottom
	"close-pane":         "\uE89F", // ClosePane
	"open-pane":          "\uE8A0", // OpenPane
	"new-window":         "\uE78B", // NewWindow
	"open-in-new-window": "\uE8A7", // OpenInNewWindow
	"minimize":           "\uE921", // ChromeMinimize
	"maximize":           "\uE922", // ChromeMaximize
	"restore":            "\uE923", // ChromeRestore
	"window-close":       "\uE8BB", // ChromeClose

	// 通用与状态
	"add":           "\uE710", // Add
	"plus":          "\uE710", // Add（别名）
	"remove":        "\uE738", // Remove
	"minus":         "\uE738", // Remove（别名）
	"check":         "\uE73E", // CheckMark
	"close":         "\uE711", // Cancel
	"more":          "\uE712", // More
	"settings":      "\uE713", // Setting
	"info":          "\uE946", // Info
	"help":          "\uE897", // Help
	"warning":       "\uE7BA", // Warning
	"error":         "\uE783", // Error
	"completed":     "\uE930", // Completed
	"filter":        "\uE71C", // Filter
	"pin":           "\uE718", // Pin
	"unpin":         "\uE77A", // Unpin
	"favorite":      "\uE734", // FavoriteStar
	"favorite-fill": "\uE735", // FavoriteStarFill
	"unfavorite":    "\uE8D9", // Unfavorite
	"share":         "\uE72D", // Share
	"link":          "\uE71B", // Link
	"attach":        "\uE723", // Attach
	"send":          "\uE724", // Send
	"mail":          "\uE715", // Mail
	"comment":       "\uE90A", // Comment
	"chat":          "\uE8F2", // ChatBubbles
	"people":        "\uE716", // People
	"contact":       "\uE77B", // Contact
	"tag":           "\uE8EC", // Tag
	"flag":          "\uE7C1", // Flag
	"bookmark":      "\uE8A4", // Bookmarks
	"library":       "\uE8F1", // Library
	"music":         "\uE90B", // MusicInfo
	"music-note":    "\uEC4F", // MusicNote
	"picture":       "\uE8B9", // Picture
	"photo":         "\uE91B", // Photo
	"camera":        "\uE722", // Camera
	"microphone":    "\uE720", // Microphone
	"network":       "\uE968", // Network
	"wifi":          "\uE701", // Wifi
	"bluetooth":     "\uE702", // Bluetooth
	"usb":           "\uE88E", // USB
	"cloud":         "\uE753", // Cloud
	"sync":          "\uE895", // Sync
	"calendar":      "\uE787", // Calendar
	"stopwatch":     "\uE916", // Stopwatch
	"calculator":    "\uE8EF", // Calculator
	"game":          "\uE7FC", // Game
	"sort":          "\uE8CB", // Sort（上下箭头）
	"download":      "\uE896", // Download
	"upload":        "\uE898", // Upload
}

// IconEntry 是内置图标表里的一项：图标名与对应的 Segoe MDL2 字形。
type IconEntry struct {
	Name  string
	Glyph string
}

// Icons 返回全部内置图标，按名称排序。
//
// 返回的是副本，修改切片不会影响内置表；别名也会作为独立条目返回。
// 图标名可以直接传给 Action.WithIcon。
func Icons() []IconEntry {
	entries := make([]IconEntry, 0, len(iconGlyphs))
	for name, glyph := range iconGlyphs {
		entries = append(entries, IconEntry{Name: name, Glyph: glyph})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
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
	return newEl(doc, "span", "system-ui-icon", g)
}
