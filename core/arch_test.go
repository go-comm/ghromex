package core_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 架构护栏：依赖方向必须是单向的。
//
//	core  ←  engine（渲染引擎，纯 Go）
//	core  ←  backend/*（平台实现）
//	engine + backend/*  ←  browser（应用层：窗口 + 文档 + 主循环）
//	engine + browser  ←  ui（组件库：菜单/工具栏/状态栏/页面组件）
//
// 平台层（backend）一旦 import 引擎，就成了「平台层依赖它上面的层」：
// 引擎的每一次改动都会牵动后端，后端也没法被别的引擎/工具复用。这个
// 测试把规则钉死，避免重构时不知不觉又倒挂回去。
func TestLayeringHasNoBackwardDependency(t *testing.T) {
	cases := []struct {
		dir string
		// allow 是该包允许依赖的仓库内包（core 自己除外）。
		allow []string
	}{
		{dir: "engine", allow: []string{"core"}},
		{dir: "backend/sdl2", allow: []string{"core"}},
		{dir: "backend/dialog", allow: nil},
		{dir: "backend/clipboard", allow: []string{"backend/sdl2"}},
		{dir: "runtime/dialog", allow: []string{"backend/dialog"}},
		{dir: "runtime/clipboard", allow: []string{"backend/clipboard"}},
		// 桌面抓屏只碰操作系统的 GDI/user32，不依赖仓库内任何渲染包：
		// 反过来让 engine/backend 依赖它就会把“截图工具的系统调用”
		// 拖进渲染链路。
		{dir: "runtime/screen", allow: nil},
		// 组件库在最上层：只用引擎（文档/DOM）与 browser（窗口）。
		// 反过来不行——引擎和 browser 都不该知道「菜单/工具栏」的存在，
		// 否则想要一个空壳窗口的应用会被迫拖上整套 UI。
		{dir: "ui", allow: []string{"engine", "browser"}},
	}
	for _, c := range cases {
		allowed := map[string]bool{"core": true}
		for _, a := range c.allow {
			allowed[a] = true
		}
		for _, imp := range repoImports(t, c.dir, false) {
			if !allowed[imp] {
				t.Errorf("%s 依赖了 %s：平台/引擎层不该反向依赖上层（允许：%v）",
					c.dir, imp, c.allow)
			}
		}
	}
}

// 浏览器层是「应用层」：它同时用引擎与后端，是唯一允许把两者缝起来的地方。
func TestBrowserLayerWiresEngineAndBackend(t *testing.T) {
	imps := map[string]bool{}
	for _, imp := range repoImports(t, "browser", false) {
		imps[imp] = true
	}
	if !imps["engine"] || !imps["backend/sdl2"] {
		t.Errorf("browser 应同时依赖 engine 与 backend/sdl2，实际：%v", imps)
	}
}

// repoImports 列出一个目录里所有 .go 文件导入的仓库内包。
//
// test=false 时只看非测试文件：测试可以按需引用引擎（如 browser 层的
// 端到端用例），但生产代码不行。
func repoImports(t *testing.T, dir string, test bool) []string {
	t.Helper()
	// 测试跑在 core 包目录下，仓库根在上一层。
	dir = filepath.Join("..", dir)
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取目录 %s: %v", dir, err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		isTest := strings.HasSuffix(name, "_test.go")
		if isTest != test {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("解析 %s: %v", filepath.Join(dir, name), err)
		}
		for _, spec := range f.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			if strings.HasPrefix(p, "github.com/go-comm/ghromex/") {
				out = append(out, strings.TrimPrefix(p, "github.com/go-comm/ghromex/"))
			}
		}
	}
	return out
}
