package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	_ "embed"

	"github.com/go-comm/ghromex/backend/sdl2"
	"github.com/go-comm/ghromex/engine"
)

//go:embed index.html
var embeddedHTML []byte

func main() {
	file := flag.String("file", "", "HTML 页面路径（默认使用内置 demo/index.html）")
	exitAfter := flag.Duration("exit-after", 0, "指定时长后自动退出，如 3s（调试/CI 用）")
	dumpSVG := flag.String("dump-svg", "", "无头模式：完成级联与布局后把结果导出为 SVG 并退出")
	flag.Parse()

	src := string(embeddedHTML)
	if *file != "" {
		data, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 HTML 失败:", err)
			os.Exit(1)
		}
		src = string(data)
	}

	if *dumpSVG != "" {
		vp := engine.NewHeadlessViewport(1024, 720)
		doc, err := engine.OpenDocument(vp, src)
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

	win, err := sdl2.NewWindow(1024, 720, "Ghromex — HTML/CSS 原生 GUI")
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
	defer win.Close()

	doc, err := win.OpenDocument(src)
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开文档失败:", err)
		os.Exit(1)
	}

	// 交互：登录/重置按钮点击更新状态文本，引擎自动重排重绘
	clicks := 0
	status := doc.QuerySelector("#status")
	if status != nil {
		if login := doc.QuerySelector("#login"); login != nil {
			login.OnClick(func(ev *engine.MouseEvent) {
				clicks++
				status.SetText(fmt.Sprintf("已点击登录 %d 次（坐标 %d, %d）", clicks, ev.X, ev.Y))
			})
		}
		if reset := doc.QuerySelector("#reset"); reset != nil {
			reset.OnClick(func(ev *engine.MouseEvent) {
				clicks = 0
				status.SetText("已重置")
			})
		}
	}

	if *exitAfter > 0 {
		win.SetAutoExit(time.Duration(*exitAfter))
	}
	if err := win.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "运行失败:", err)
		os.Exit(1)
	}
}
