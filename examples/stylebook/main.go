// 示例：作者样式表 + 全部支持的标签与 CSS（对照 Chrome，不看 UA 默认）。
//
//	go run ./examples/stylebook
//	go run ./examples/stylebook -exit-after 3s     # 调试/CI
//	go run ./examples/stylebook -dump-png out.png  # 无头出图：软件光栅 PNG，文字是占位方块
//	go run ./examples/stylebook -dump-svg out.svg  # 无头出图：级联+布局导出 SVG，核对几何/配色
//	go run ./examples/stylebook -shot out.png      # 真实窗口截图：真实字体渲染一帧
//
// index.html 与 stylebook.css 同目录 go:embed：页面里的
// <link rel="stylesheet" href="stylebook.css"> 由本文件的加载器接住——引擎
// 不碰 I/O，只把 href 解析成绝对地址回调宿主。同一份 index.html 用 Chrome
// 直接打开（start examples\stylebook\index.html）读的是磁盘上同一份 CSS，
// 两边逐节对照。
//
// 与 examples/ua 的分工：ua 页是零作者 CSS，检验引擎内置 UA 默认样式；
// 本页反过来——所有样式都来自作者样式表，逐节摆出引擎目前支持的全部标签
// 与 CSS 能力，与 Chrome 对照作者样式的解析、级联与绘制。
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"time"

	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
)

//go:embed index.html
var page []byte

//go:embed stylebook.css
var stylesheet []byte

func main() {
	exitAfter := flag.Duration("exit-after", 0, "指定时长后自动退出，如 3s（调试/CI 用）")
	dumpPNG := flag.String("dump-png", "", "无头出图：软件光栅 PNG，文字是占位方块，断言布局/配色用")
	dumpSVG := flag.String("dump-svg", "", "无头出图：级联+布局导出 SVG（真实 <text>），核对几何/层级/配色")
	shot := flag.String("shot", "", "真实窗口截图：真实字体渲染一帧存 PNG（Window.Screenshot 回读）")
	flag.Parse()

	if *dumpPNG != "" || *dumpSVG != "" {
		vp, buf := engine.NewBufferedViewport(1120, 820)
		doc, err := engine.OpenDocumentWith(vp, string(page), docOptions())
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

	win, err := browser.New(1120, 820, "Ghromex — 样式全集（作者样式表）")
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
	defer win.Close()

	doc, err := win.OpenDocumentWith(string(page), docOptions())
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开文档失败:", err)
		os.Exit(1)
	}

	if *shot != "" {
		if err := saveShot(win, doc, *shot); err != nil {
			fmt.Fprintln(os.Stderr, "截图失败:", err)
			os.Exit(1)
		}
		fmt.Println("PNG 已写入:", *shot)
		return
	}

	if *exitAfter > 0 {
		win.SetAutoExit(time.Duration(*exitAfter))
	}
	if err := win.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "运行失败:", err)
		os.Exit(1)
	}
}

// docOptions 返回打开本页的文档选项：虚拟的页面地址 + 只认本页样式表的
// 加载器。
//
// BaseURL 是虚拟的（页面内容来自 go:embed，磁盘上没有对应文件）：写成
// 本示例目录的 index.html，相对引用 stylebook.css 解析出的地址就与「页面
// 真的在磁盘上」一致；加载器只按解析结果的尾名认这份内嵌样式表，其余一律
// 报错——样式「为什么不生效」总得有个地方能看见（引擎收到错误会忽略该表
// 并继续渲染）。
func docOptions() engine.DocumentOptions {
	return engine.DocumentOptions{
		BaseURL: "file:///examples/stylebook/index.html",
		LoadStylesheet: func(href string) (string, error) {
			if strings.HasSuffix(href, "stylebook.css") {
				return string(stylesheet), nil
			}
			err := fmt.Errorf("示例只内嵌了 stylebook.css，跳过 %s", href)
			fmt.Fprintln(os.Stderr, "样式表加载失败:", err)
			return "", err
		},
	}
}

// saveShot 用真实字体渲染一帧并截图：Refresh 重排、RenderFrame 画一帧、
// Window.Screenshot 回读窗口像素后本地编码 PNG。
//
// 与 -dump-png 的区别：那条路用软件光栅后端，文字是占位方块（用途是逐
// 像素断言）；这条走真实字体与 SDL 渲染器，用来看最终观感。
func saveShot(win *browser.Window, doc engine.HTMLDocument, path string) error {
	// OpenDocument 已布局，但先 Refresh 再画，口径与主循环一致
	// （RenderFrame 只负责「清屏 + 画当前布局」，不自己重排）。
	if doc != nil {
		doc.Refresh()
	}
	win.RenderFrame()
	raw, _, w, h, err := win.Screenshot()
	if err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// 后端回读格式为 XBGR8888（小端内存序 R,G,B,X）：前三个字节就是
	// 颜色，第 4 字节不是 alpha，统一补 255。
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			o := y*img.Stride + x*4
			img.Pix[o] = raw[i]
			img.Pix[o+1] = raw[i+1]
			img.Pix[o+2] = raw[i+2]
			img.Pix[o+3] = 255
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
