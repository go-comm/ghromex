// 本文件把引擎的「外部资源」能力接到本地文件系统上：地址换算 + 只读本地
// 文件的样式表/图片加载器。包文档见 main.go。
//
// 引擎（engine 包）不碰 I/O：它只把 <link rel="stylesheet" href>、CSS 的
// @import 与 <img src> 解析成绝对地址，再回调宿主给的加载器。本示例的宿主
// 就是这里——只认 file:// 地址，不联网（与页面导航的口径一致）。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// pageBaseURL 返回页面的地址基准：外部样式表的相对引用按它解析。
//
// 地址可能是虚拟的：起始页来自 go:embed，磁盘上没有对应文件
// （addr = about:home）。此时用所在目录下的 index.html 当名义地址，
// 相对引用的解析结果与「页面真的在磁盘上」一致；目录都没找到时，
// 相对链接本来就走错误页，这里同样退化成相对当前工作目录。
func pageBaseURL(addr, dir string) string {
	if addr == "" || strings.HasPrefix(addr, "about:") {
		return fileURL(filepath.Join(dir, "index.html"))
	}
	if strings.HasPrefix(addr, "file://") {
		return addr
	}
	return fileURL(addr)
}

// fileURL 把本地路径转成 file:// 地址（引擎按 URL 口径解析相对引用，
// 所以 base 必须是 URL，不能是 Windows 路径）。
//
//	examples/browser/index.html  →  file:///…/examples/browser/index.html
func fileURL(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path // 取不到绝对路径就原样用：相对目录下也能解析
	}
	u := filepath.ToSlash(abs)
	if !strings.HasPrefix(u, "/") {
		u = "/" + u // C:/x → /C:/x，与 file:///C:/x 的写法对齐
	}
	return "file://" + u
}

// pathFromFileURL 把 file:// 地址还原成本地路径。
//
// 只处理本机文件：file://host/share 是 UNC（网络共享），本示例不支持，
// 返回 ok=false 让加载器按失败处理——总比把它当成本地相对路径读错文件好。
func pathFromFileURL(u string) (string, bool) {
	if len(u) < 5 || !strings.EqualFold(u[:5], "file:") {
		return "", false
	}
	rest := u[5:]
	if strings.HasPrefix(rest, "//") {
		rest = rest[2:]
		if !strings.HasPrefix(rest, "/") {
			return "", false
		}
	}
	// 地址里的 query/fragment 不是文件路径的一部分。
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	if rest == "" || rest == "/" {
		return "", false
	}
	// file:///C:/x → /C:/x → C:/x（Windows 盘符前的斜杠不属于路径）
	if len(rest) >= 3 && rest[0] == '/' && rest[2] == ':' {
		rest = rest[1:]
	}
	return filepath.FromSlash(rest), true
}

// loadLocalImage 是示例的图片加载器：与样式表同一口径，只读本地文件。
//
// 引擎在 worker goroutine 里调用它（<img> 异步加载），本函数只做纯读取，
// 天然线程安全。失败只打到 stderr：引擎会把该图片按空图处理，页面继续渲染。
func loadLocalImage(src string) ([]byte, error) {
	if isExternal(src) {
		err := fmt.Errorf("示例不联网，跳过外部图片 %s", src)
		fmt.Fprintln(os.Stderr, "图片加载失败:", err)
		return nil, err
	}
	path, ok := pathFromFileURL(src)
	if !ok {
		err := fmt.Errorf("不支持的图片地址 %s", src)
		fmt.Fprintln(os.Stderr, "图片加载失败:", err)
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "图片加载失败:", err)
		return nil, err
	}
	return data, nil
}

// loadLocalStylesheet 是示例的样式表加载器：只读本地文件，不联网。
//
// 失败时返回 error——引擎会忽略这份样式表并继续渲染。这里同时打到 stderr：
// 示例没有 DevTools，样式「为什么不生效」总得有个地方能看见。
func loadLocalStylesheet(href string) (string, error) {
	if isExternal(href) {
		err := fmt.Errorf("示例不联网，跳过外部样式表 %s", href)
		fmt.Fprintln(os.Stderr, "样式表加载失败:", err)
		return "", err
	}
	path, ok := pathFromFileURL(href)
	if !ok {
		err := fmt.Errorf("不支持的样式表地址 %s", href)
		fmt.Fprintln(os.Stderr, "样式表加载失败:", err)
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "样式表加载失败:", err)
		return "", err
	}
	return string(data), nil
}
