package engine

import "testing"

// TestResolveURL 钉住相对引用的解析规则：<link> 与 @import 都靠它把
// 「相对谁」算准，算错的表现是「样式表 404 / 读错目录」，而不是报错。
func TestResolveURL(t *testing.T) {
	cases := []struct {
		base, ref, want string
	}{
		// 同目录 / 子目录 / 上级目录
		{"file:///site/page.html", "theme.css", "file:///site/theme.css"},
		{"file:///site/page.html", "css/theme.css", "file:///site/css/theme.css"},
		{"file:///site/css/main.css", "../base.css", "file:///site/base.css"},
		{"file:///site/css/main.css", "./x/./y.css", "file:///site/css/x/y.css"},
		// Windows 盘符（file:///C:/… 的 path 是 /C:/…）
		{"file:///C:/dir/page.html", "a.css", "file:///C:/dir/a.css"},
		{"file:///C:/dir/css/main.css", "../a.css", "file:///C:/dir/a.css"},
		// 根相对：只换 path，保留 scheme/authority
		{"file:///a/b/c.html", "/x.css", "file:///x.css"},
		{"https://site/a/b", "/x.css", "https://site/x.css"},
		// 绝对地址原样返回（scheme 语义留给宿主）
		{"file:///a/b", "https://cdn/x.css", "https://cdn/x.css"},
		{"file:///a/b", "http://cdn/x.css", "http://cdn/x.css"},
		// 协议相对：继承 base 的 scheme
		{"https://site/a/b", "//cdn/x.css", "https://cdn/x.css"},
		// 上级越界：停在根
		{"file:///a.css", "../x.css", "file:///x.css"},
		// 没有基准：原样返回（由加载器按自己的默认处理）
		{"", "x.css", "x.css"},
		{"", "https://cdn/x.css", "https://cdn/x.css"},
		// 空引用
		{"file:///a/b", "", ""},
		{"file:///a/b", "   ", ""},
		// 引用两端的空白要去掉（CSS 里常见 url( "x.css" ) 的写法）
		{"file:///a/b", "  x.css  ", "file:///a/x.css"},
		// 裸路径当 base（没有 scheme）：按相对路径拼接，不能拼出 "://"
		{"dir/page.html", "a.css", "dir/a.css"},
	}
	for _, c := range cases {
		if got := resolveURL(c.base, c.ref); got != c.want {
			t.Errorf("resolveURL(%q, %q) = %q, want %q", c.base, c.ref, got, c.want)
		}
	}
}

// TestHasURLScheme 单字母不算 scheme：Windows 盘符（C:/x.css）不能被当成
// 「带 scheme 的绝对地址」跳过解析。
func TestHasURLScheme(t *testing.T) {
	for _, s := range []string{"http://x", "https://x", "file:///x", "data:text/css,x"} {
		if !hasURLScheme(s) {
			t.Errorf("hasURLScheme(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"C:/x.css", "a.css", "/x.css", "//cdn/x.css", "", "1:x"} {
		if hasURLScheme(s) {
			t.Errorf("hasURLScheme(%q) = true, want false", s)
		}
	}
}
