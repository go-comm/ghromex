//go:build windows

// 示例：滚动与裁剪（文档级滚动、overflow 容器、textarea 滚轮、wheel 事件）。
// 直接运行：
//
//	go run ./examples/scroll
//	go run ./examples/scroll -exit-after 3s     # 调试/CI
//	go run ./examples/scroll -dump-svg out.svg  # 无头导出
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
)

//go:embed index.html
var page []byte

func main() {
	exitAfter := flag.Duration("exit-after", 0, "指定时长后自动退出，如 3s（调试/CI 用）")
	dumpSVG := flag.String("dump-svg", "", "无头模式：完成级联与布局后把结果导出为 SVG 并退出")
	flag.Parse()

	if *dumpSVG != "" {
		vp := engine.NewHeadlessViewport(1024, 720)
		doc, err := engine.OpenDocument(vp, string(page))
		if err != nil {
			fmt.Fprintln(os.Stderr, "打开文档失败:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*dumpSVG, []byte(doc.DumpSVG()), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("SVG 已写入:", *dumpSVG)
		return
	}

	win, err := browser.New(1024, 720, "Ghromex — 滚动与裁剪")
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
	defer win.Close()

	doc, err := win.OpenDocument(string(page))
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开文档失败:", err)
		os.Exit(1)
	}

	// 交互：#banner 回显最近一次 wheel 位移
	if banner := doc.QuerySelector("#banner"); banner != nil {
		banner.OnWheel(func(ev *engine.MouseEvent) {
			banner.SetText(fmt.Sprintf("wheel 位移 dx=%d dy=%d @(%d,%d)", ev.DeltaX, ev.DeltaY, ev.X, ev.Y))
		})
	}

	if *exitAfter > 0 {
		win.SetAutoExit(time.Duration(*exitAfter))
	}
	if err := win.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "运行失败:", err)
		os.Exit(1)
	}
}
