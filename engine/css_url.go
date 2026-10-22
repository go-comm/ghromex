package engine

import "strings"

// resolveURL 把样式表引用 ref 按引用方地址 base 解析成绝对地址。
//
// 这里只做**纯字符串**解析：不碰文件系统、不发网络请求——scheme 的语义
// 留给宿主（file:// 就读文件、https:// 就发请求），引擎只负责把「相对
// 谁」算准。之所以由引擎算而不是让宿主算：@import 可以嵌套，第二层以后
// 的基准是**上一层样式表的地址**而不是页面地址；宿主每层自己拼路径时，
// 少传一个基准就会把子目录里的引用解析错，而且这种错很难看出来。
//
// 支持的形式（CSS 里实际会出现的）：
//   - 带 scheme 的绝对地址（http://、https://、file://…）→ 原样返回；
//   - 协议相对（//host/x.css）→ 继承 base 的 scheme；
//   - 根相对（/css/a.css）→ 换掉 base 的 path；
//   - 相对（css/a.css、./a.css、../a.css）→ 拼到 base 的目录后归一化。
//
// base 为空时原样返回 ref：没有基准就无从解析，交给加载器按自己的默认
// 处理（例如把它当绝对文件路径）。地址按 URL 口径用 `/` 分隔；纯文件路径
// 请宿主先转成 file:// 地址再传进来（见 examples/browser 的 fileURL）。
func resolveURL(base, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if hasURLScheme(ref) {
		return ref
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return ref
	}
	if strings.HasPrefix(ref, "//") {
		return urlScheme(base) + ref
	}
	scheme, authority, path := splitURL(base)
	// 没有 scheme 的 base（宿主传了裸路径）按相对路径处理，不能再加 "://"。
	prefix := ""
	if scheme != "" {
		prefix = scheme + "://" + authority
	}
	if strings.HasPrefix(ref, "/") {
		return prefix + cleanURLPath(ref)
	}
	dir := ""
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		dir = path[:i+1]
	}
	return prefix + cleanURLPath(dir+ref)
}

// hasURLScheme 判断 s 是否以 scheme: 开头。单字母不算：Windows 盘符
// （C:/x.css）长得像 scheme，但只有单个字母，按绝对地址原样返回更安全。
func hasURLScheme(s string) bool {
	colon := strings.IndexByte(s, ':')
	if colon < 2 {
		return false
	}
	for i := 0; i < colon; i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case i > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'):
		default:
			return false
		}
	}
	return true
}

// urlScheme 返回 base 的 scheme（带冒号，如 "https:"）；没有则空串。
func urlScheme(base string) string {
	colon := strings.Index(base, "://")
	if colon <= 0 {
		return ""
	}
	return base[:colon+1]
}

// splitURL 把 URL 拆成 scheme / authority / path 三段。
// 例：https://cdn/x.css → ("https", "cdn", "/x.css")；
//
//	file:///C:/a.css → ("file", "", "/C:/a.css")。
func splitURL(u string) (scheme, authority, path string) {
	rest := u
	if i := strings.Index(rest, "://"); i >= 0 {
		scheme = rest[:i]
		rest = rest[i+3:]
	}
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		authority = rest[:i]
		path = rest[i:]
	} else {
		authority = rest
		path = "/"
	}
	if scheme == "" {
		// 没有 scheme 的 base（调用方传了裸路径）：整串当 path 用，
		// 这样 "dir/page.html" 这类调用也能得到 "dir/x.css"。
		return "", "", u
	}
	return scheme, authority, path
}

// cleanURLPath 归一化 path：合并重复的 `/`、消掉 `.` 与 `..`、保留开头的
// `/`（根相对）与结尾的 `/`（目录语义）。
func cleanURLPath(p string) string {
	lead := strings.HasPrefix(p, "/")
	trail := strings.HasSuffix(p, "/")
	var segs []string
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "", ".":
			continue
		case "..":
			if len(segs) > 0 {
				segs = segs[:len(segs)-1]
			}
		default:
			segs = append(segs, seg)
		}
	}
	out := strings.Join(segs, "/")
	if lead {
		out = "/" + out
	}
	if trail && out != "" && !strings.HasSuffix(out, "/") {
		out += "/"
	}
	return out
}
