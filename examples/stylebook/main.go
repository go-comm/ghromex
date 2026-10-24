// 示例：作者样式表 + 全部支持的标签与 CSS（对照 Chrome，不看 UA 默认）。
//
//	go run ./examples/stylebook
//	go run ./examples/stylebook -exit-after 3s     # 调试/CI
//	go run ./examples/stylebook -dump-png out.png  # 无头出图：软件光栅 PNG，文字是占位方块
//	go run ./examples/stylebook -dump-svg out.svg  # 无头出图：级联+布局导出 SVG，核对几何/配色
//	go run ./examples/stylebook -shot out.png      # 真实窗口整页截图：真实字体，超长页分段拼接
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
	"image/draw"
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
	shot := flag.String("shot", "", "真实窗口整页截图：真实字体，超长页分段回读拼接存 PNG")
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
		// 整页截图：视口截图只能拍到首屏；文档超过单段上限时分段回读拼接
		// （见 saveShotFull），段高不足的窗口先拉起。
		if err := saveShotFull(win, doc, *shot); err != nil {
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

// pageHeight 估算文档总高：末节底边 + body 下边距（8px）与卡片外边距。
// 用末节而非 body：语义标签块级化与否都会让 body 盒高度可信，取到末节
// 底边更稳（页面后续加节只需换选择器）。
func pageHeight(doc engine.HTMLDocument) int {
	const last = "sec-unsupported"
	el := doc.GetElementById(last)
	if el == nil {
		return 0
	}
	return el.GetBoundingClientRect().Bottom().Pixel() + 40
}

// shotSegMax 是单段截图的最大高度：SDL 渲染目标（render target 纹理）的
// 高度上限是 8192——窗口拉到 8681 时 8192 以下有内容、以上整段黑，实测
// 而得；留 192px 裕量取 8000。
const shotSegMax = 8000

// saveShotFull 真实字体整页截图：窗口按段高拉起、逐段回读，纵向拼成一张。
//
// 段偏移用 body 的 position:relative + 负 top 平移绘制（只动绘制位置，
// 布局树不变，段间互不影响）。position:fixed 的 .pos-fix 不随 body 平移：
// 只有「段底 == 页底」的最后一段能把它画在正确位置（段视口底 = 页底），
// 其余段先 display:none 藏掉，末段再 display:inline 放回来——同一选择器
// 后写的规则级联取胜，顺序即正确性，不能调换。
func saveShotFull(win *browser.Window, doc engine.HTMLDocument, path string) error {
	total := pageHeight(doc)
	if total <= 0 {
		total = win.ViewportHeight()
	}
	w := win.ViewportWidth()
	out := image.NewRGBA(image.Rect(0, 0, w, total))

	for off := 0; off < total; off += shotSegMax {
		segH := shotSegMax
		if off+segH > total {
			segH = total - off
		}
		var css string
		if off > 0 {
			css += fmt.Sprintf("body{position:relative;top:%dpx}", -off)
		}
		if off+segH < total {
			css += ".pos-fix{display:none}"
		} else if off > 0 {
			css += ".pos-fix{display:inline}"
		}
		if css != "" {
			doc.AddStylesheetText(css)
		}
		if win.ViewportHeight() != segH {
			win.Resize(w, segH)
		}
		doc.Refresh()
		win.RenderFrame()
		raw, _, sw, sh, err := win.Screenshot()
		if err != nil {
			return err
		}
		if sw != w {
			return fmt.Errorf("段截图宽 = %d, want %d", sw, w)
		}
		if off+sh > total {
			sh = total - off
		}
		draw.Draw(out, image.Rect(0, off, w, off+sh), rawToRGBA(raw, sw, sh), image.Point{}, draw.Src)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, out)
}

// rawToRGBA 把后端回读的 XBGR8888（小端内存序 R,G,B,X）转成 RGBA：前三个
// 字节就是颜色，第 4 字节不是 alpha，统一补 255。
func rawToRGBA(raw []byte, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
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
	return img
}
