package ui

import (
	"testing"
	"unicode/utf8"
)

// 图标表要覆盖常用命令；按用途分组检查，新增分类时不容易漏。
func TestIconSetCoversCommandGroups(t *testing.T) {
	groups := map[string][]string{
		"文件/编辑": {
			"new", "open", "save", "save-as", "copy", "cut", "paste",
			"undo", "redo", "search", "replace", "print", "delete", "refresh",
		},
		"方向/导航": {
			"up", "down", "left", "right",
			"chevron-up", "chevron-down", "chevron-left", "chevron-right",
			"page-left", "page-right", "go-to-start", "back", "forward", "home",
		},
		"对齐/排版": {
			"align-left", "align-center", "align-right",
			"bulleted-list", "list", "check-list",
			"bold", "italic", "underline", "font-size", "font-color", "highlight",
		},
		"播放/媒体": {
			"play", "pause", "stop", "record", "previous", "next",
			"rewind", "fast-forward", "shuffle", "repeat", "repeat-one",
			"volume", "mute", "fullscreen", "back-to-window",
		},
		"视图/窗口": {
			"zoom-in", "zoom-out", "fit-page", "view", "view-all", "preview",
			"dock-left", "dock-right", "dock-bottom", "close-pane", "open-pane",
			"minimize", "maximize", "restore", "window-close",
		},
		"通用": {
			"check", "close", "add", "remove", "more", "filter", "settings",
			"info", "help", "warning", "error", "favorite", "share", "link",
			"attach", "send", "folder", "calendar",
		},
	}
	for group, names := range groups {
		for _, name := range names {
			glyph, ok := iconGlyphs[name]
			if !ok || glyph == "" {
				t.Errorf("%s：缺少图标 %q", group, name)
			}
		}
	}
}

// 用户最常点名的几组图标要落到正确码位，防止改名/改码位时悄悄错位。
func TestIconSetCodepoints(t *testing.T) {
	want := map[string]string{
		"up":             "\uE74A",
		"down":           "\uE74B",
		"left":           "\uE72B",
		"right":          "\uE72A",
		"chevron-up":     "\uE70E",
		"chevron-down":   "\uE70D",
		"chevron-left":   "\uE76B",
		"chevron-right":  "\uE76C",
		"align-left":     "\uE8E4",
		"align-center":   "\uE8E3",
		"align-right":    "\uE8E2",
		"play":           "\uE768",
		"pause":          "\uE769",
		"stop":           "\uE71A",
		"record":         "\uE7C8",
		"previous":       "\uE892",
		"next":           "\uE893",
		"rewind":         "\uEB9E",
		"fast-forward":   "\uEB9D",
		"shuffle":        "\uE8B1",
		"repeat":         "\uE8EE",
		"repeat-one":     "\uE8ED",
		"volume":         "\uE767",
		"mute":           "\uE74F",
		"fullscreen":     "\uE740",
		"back-to-window": "\uE73F",
		"font-size":      "\uE8E9",
		"font-color":     "\uE8D3",
		"zoom-in":        "\uE8A3",
		"fit-page":       "\uE9A6",
	}
	for name, wantGlyph := range want {
		if got := iconGlyphs[name]; got != wantGlyph {
			t.Errorf("%s 字形 = %q，期望 %q", name, got, wantGlyph)
		}
	}
}

// 别名必须指向同一字形；left/right 与 back/forward 本来就是同一个箭头。
func TestIconAliases(t *testing.T) {
	aliases := map[string]string{
		"arrow-up":    "up",
		"arrow-down":  "down",
		"arrow-left":  "left",
		"arrow-right": "right",
		"open-file":   "open",
		"trash":       "delete",
		"reload":      "refresh",
		"printer":     "print",
		"plus":        "add",
		"minus":       "remove",
	}
	for alias, target := range aliases {
		got, want := iconGlyphs[alias], iconGlyphs[target]
		if got == "" || got != want {
			t.Errorf("%s 字形 = %q，期望与 %s 相同（%q）", alias, got, target, want)
		}
	}
	if iconGlyphs["left"] != iconGlyphs["back"] {
		t.Error("left 应与 back 同为左箭头")
	}
	if iconGlyphs["right"] != iconGlyphs["forward"] {
		t.Error("right 应与 forward 同为右箭头")
	}
}

// Icons 暴露整张表：数量一致、按名称排序、字形与内置表相同，且是副本。
func TestIcons(t *testing.T) {
	entries := Icons()
	if len(entries) == 0 {
		t.Fatal("Icons 返回空表")
	}
	if len(entries) != len(iconGlyphs) {
		t.Fatalf("Icons 条目数 = %d，期望 %d", len(entries), len(iconGlyphs))
	}
	for i, e := range entries {
		if i > 0 && entries[i-1].Name >= e.Name {
			t.Fatalf("Icons 未按名称排序：%q 在 %q 之后", e.Name, entries[i-1].Name)
		}
		if want := iconGlyphs[e.Name]; e.Glyph != want {
			t.Errorf("%s 字形 = %q，期望 %q", e.Name, e.Glyph, want)
		}
	}

	// 返回值不能是内置表的别名（当前 map 本来就不是切片，这里防的是
	// 以后改成包级切片缓存时把内部状态漏出去）。
	other := Icons()
	entries[0].Name = "changed"
	if _, ok := iconGlyphs["changed"]; ok {
		t.Error("修改 Icons 返回值不应影响内置表")
	}
	if other[0].Name == "changed" {
		t.Error("两次 Icons 调用不应共享同一个切片")
	}
}

// 内置表里的值必须是私用区的单个字形；多字符会把图标画成文字。
func TestIconGlyphsAreSinglePUA(t *testing.T) {
	for name, glyph := range iconGlyphs {
		if n := utf8.RuneCountInString(glyph); n != 1 {
			t.Errorf("%s 字形 %q 含 %d 个字符，期望 1", name, glyph, n)
			continue
		}
		r, _ := utf8.DecodeRuneInString(glyph)
		if r < 0xE000 || r > 0xF8FF {
			t.Errorf("%s 字形 %q = U+%04X，不在 Unicode 私用区", name, glyph, r)
		}
	}
}
