// 示例：原生 UA 表单 + 常用标签（零作者 CSS，可与 Chrome/Edge 对照）。直接运行：
//
//	go run ./examples/ua
//	go run ./examples/ua -exit-after 3s     # 调试/CI
//	go run ./examples/ua -dump-png out.png  # 无头出图：软件光栅 PNG
//	go run ./examples/ua -dump-svg out.svg  # 无头出图：SVG，核对布局/层级
//	go run ./examples/ua -debug-port 9241   # 本地调试服务：HTTP/JSON 驱动与观测
//
// 页面 index.html 与本文件同目录，用 go:embed 打进二进制。
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
	"github.com/go-comm/ghromex/examples/internal/debugsrv"
)

//go:embed index.html
var page []byte

func main() {
	exitAfter := flag.Duration("exit-after", 0, "指定时长后自动退出，如 3s（调试/CI 用）")
	dumpPNG := flag.String("dump-png", "", "无头出图：软件光栅 PNG，文字是占位方块，断言布局/配色用")
	dumpSVG := flag.String("dump-svg", "", "无头出图：级联+布局导出 SVG（真实 <text>），核对几何/层级/配色")
	debugPort := debugsrv.Flag()
	flag.Parse()

	if *dumpPNG != "" || *dumpSVG != "" {
		vp, buf := engine.NewBufferedViewport(1024, 720)
		doc, err := engine.OpenDocument(vp, string(page))
		if err != nil {
			fmt.Fprintln(os.Stderr, "打开文档失败:", err)
			os.Exit(1)
		}
		// 两个出口共用一份布局：先光栅化（要 PNG 才画），再出 SVG。
		if *dumpPNG != "" {
			engine.RenderNode(buf, doc)
			if err := buf.DumpPNG(*dumpPNG); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Println("PNG 已写入:", *dumpPNG)
		}
		if *dumpSVG != "" {
			if err := os.WriteFile(*dumpSVG, []byte(doc.DumpSVG()), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Println("SVG 已写入:", *dumpSVG)
		}
		return
	}

	win, err := browser.New(1024, 720, "Ghromex — 原生 UA 与常用标签")
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
	defer win.Close()

	// 控件由引擎内建的 UA 样式与交互逻辑驱动，本页无需额外注册事件
	if _, err := win.OpenDocument(string(page)); err != nil {
		fmt.Fprintln(os.Stderr, "打开文档失败:", err)
		os.Exit(1)
	}

	if *exitAfter > 0 {
		win.SetAutoExit(time.Duration(*exitAfter))
	}
	// 调试服务：-debug-port 优先，留空则看环境变量 GHROMEX_DEBUG_PORT。
	stopDebug := debugsrv.Serve(win, *debugPort)
	defer stopDebug()
	if err := win.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "运行失败:", err)
		os.Exit(1)
	}
}
