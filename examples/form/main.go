//go:build windows

// 示例：常规表单（员工登记表），演示定位/层叠/盒模型与可输入控件。直接运行：
//
//	go run ./examples/form
//	go run ./examples/form -exit-after 3s     # 调试/CI
//	go run ./examples/form -dump-svg out.svg  # 无头导出
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

	win, err := browser.New(1024, 720, "Ghromex — 常规表单")
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
	defer win.Close()

	// 控件由引擎内建的聚焦/输入逻辑驱动，本页无需额外注册事件
	if _, err := win.OpenDocument(string(page)); err != nil {
		fmt.Fprintln(os.Stderr, "打开文档失败:", err)
		os.Exit(1)
	}

	if *exitAfter > 0 {
		win.SetAutoExit(time.Duration(*exitAfter))
	}
	if err := win.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "运行失败:", err)
		os.Exit(1)
	}
}
