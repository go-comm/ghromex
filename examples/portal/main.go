// 示例：购物门户（GHROMEX MARKET），演示固定顶栏、Hero、分类、秒杀、
// 商品网格、绝对定位侧栏与页脚。直接运行：
//
//	go run ./examples/portal
//	go run ./examples/portal -exit-after 3s     # 调试/CI
//	go run ./examples/portal -dump-svg out.svg  # 无头出图：SVG，核对布局/层级
//
// 页面 index.html 与本文件同目录，用 go:embed 打进二进制，因此从任意
// 工作目录运行都能出画面，不依赖当前目录。
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
)

//go:embed index.html
var page []byte

func main() {
	exitAfter := flag.Duration("exit-after", 0, "指定时长后自动退出，如 3s（调试/CI 用）")
	dumpSVG := flag.String("dump-svg", "", "无头出图：级联+布局导出 SVG（真实 <text>），核对几何/层级/配色")
	flag.Parse()

	if *dumpSVG != "" {
		vp := engine.NewHeadlessViewport(1280, 820)
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

	win, err := browser.New(1280, 820, "Ghromex · 购物门户")
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
	wirePage(doc)

	if *exitAfter > 0 {
		win.SetAutoExit(time.Duration(*exitAfter))
	}
	if err := win.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "运行失败:", err)
		os.Exit(1)
	}
}

// wirePage 给纯展示页面加两处轻量交互，证明引擎能承载真实门户的点击路径：
//   - 搜索按钮读取输入框当前值，并把提示文字改成搜索反馈；
//   - 所有 .add 按钮共用一个购物车计数，点击后同步顶栏数字与侧栏文案。
//
// 这里不引入 JS：示例页面由 Go 注册 OnClick，符合本项目「HTML/CSS 写界面、
// Go 写行为」的分工。空查询不产生购物车副作用，只恢复热搜提示。
func wirePage(doc engine.HTMLDocument) {
	search := doc.GetElementById("search")
	tip := doc.GetElementById("search-tip")
	if btn := doc.GetElementById("search-btn"); btn != nil {
		btn.OnClick(func(*engine.MouseEvent) {
			if tip == nil {
				return
			}
			query := strings.TrimSpace(engine.GetValue(search))
			if query == "" {
				tip.SetText("热门：降噪耳机 / 4K 显示器 / 机械键盘")
				return
			}
			tip.SetText("已搜索「" + query + "」 · 演示页展示静态商品")
		})
	}

	count := 3
	countEl := doc.GetElementById("cart-count")
	noteEl := doc.GetElementById("cart-note")
	for _, btn := range doc.QuerySelectorAll(".add") {
		btn.OnClick(func(*engine.MouseEvent) {
			count++
			if countEl != nil {
				countEl.SetText(strconv.Itoa(count))
			}
			if noteEl != nil {
				noteEl.SetText("购物车已有 " + strconv.Itoa(count) + " 件 · 满 299 包邮")
			}
		})
	}
}
