// stressgen 生成滚动压测页：14.9MB HTML / 约 42 万元素的长页面，用来量大文档
// 的滚动帧成本（README「大文档滚动」那节的数字就是这么来的）。
//
// 为什么需要它：滚动期的瓶颈与节点总数线性相关，门户页那种 3k 节点的页面
// 量不出任何东西——两页绘制调用数同量级（981 vs 1183），重绘却差 198 倍
// （198ms vs 1.00ms）。要看见就得有一页真正重的。
//
// 用法：
//
//	go run ./examples/stressgen -out .temp/stress.html
//	go build -o .temp/browser.exe ./examples/browser
//	GHROMEX_SCROLLPROBE=1 GHROMEX_SCROLLPROBE_EVERY=8 GHROMEX_SCROLLPROBE_NOTCH=4 \
//	  .temp/browser.exe -page .temp/stress.html -exit-after 40s
//
// 页面本身是内容页（标题 + 段落 + 表格 + 列表 + 标签），刻意不写 :hover、
// 不放图片、不用表单：这样滚动期的开销全在「遍历 + 布局 + 绘制」，不会被
// 悬停重排、纹理上传、控件布局抢走注意力（这些各自另有实测，见 CHANGELOG）。
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	out := flag.String("out", ".temp/stress.html", "输出路径")
	sections := flag.Int("sections", 6000, "节数（每节约 21 个元素）")
	para := flag.Int("para", 8, "每节段落数")
	lists := flag.Int("li", 5, "每节列表项数")
	flag.Parse()

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建输出文件失败:", err)
		os.Exit(1)
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	defer w.Flush()

	fmt.Fprintln(w, `<!doctype html><html><head><meta charset="utf-8"><title>stress</title>`)
	fmt.Fprintln(w, `<style>`)
	fmt.Fprintln(w, `body{font:14px/1.6 "Segoe UI","Microsoft YaHei",sans-serif;margin:0;padding:16px;background:#f6f7f9;color:#1f2328}`)
	fmt.Fprintln(w, `section{background:#fff;border:1px solid #d6dae1;border-radius:6px;padding:14px 16px;margin:0 0 14px}`)
	fmt.Fprintln(w, `h2{margin:0 0 8px;font-size:20px;color:#0b5cad}`)
	fmt.Fprintln(w, `p{margin:0 0 6px}`)
	fmt.Fprintln(w, `ul{margin:0 0 6px;padding-left:20px}li{margin:0 0 2px}`)
	fmt.Fprintln(w, `.tag{display:inline-block;padding:1px 6px;border:1px solid #c8ccd4;border-radius:10px;background:#eef2f7;font-size:12px}`)
	fmt.Fprintln(w, `table{border-collapse:collapse;margin:0 0 6px}td,th{border:1px solid #ccd2da;padding:3px 8px}`)
	fmt.Fprintln(w, `</style></head><body>`)
	fmt.Fprintln(w, `<h1>滚动压力页</h1>`)

	lorem := "引擎的滚动只平移已布局的盒子，不重排；这条快路径的收益要靠大页面才看得出来。" +
		"Continuous scrolling should not trigger cascade or layout work."
	row := strings.Repeat("列数据 ", 6)

	for i := 0; i < *sections; i++ {
		fmt.Fprintf(w, `<section><h2>第 %d 节 · 条目明细</h2><span class="tag">标签%d</span><span class="tag">状态%d</span>`,
			i+1, i%7, i%3)
		for p := 0; p < *para; p++ {
			fmt.Fprintf(w, `<p>%s（%d-%d）</p>`, lorem, i+1, p+1)
		}
		fmt.Fprint(w, `<table><tr><th>字段</th><th>值</th><th>说明</th></tr>`)
		for r := 0; r < 3; r++ {
			fmt.Fprintf(w, `<tr><td>%s%d</td><td>%d</td><td>%s</td></tr>`, "名称", r, i*10+r, row)
		}
		fmt.Fprint(w, `</table><ul>`)
		for l := 0; l < *lists; l++ {
			fmt.Fprintf(w, `<li>要点 %d.%d：滚动、层叠与定位的口径要在绘制与命中测试之间保持一致</li>`, i+1, l+1)
		}
		fmt.Fprintln(w, `</ul></section>`)
	}
	fmt.Fprintln(w, `</body></html>`)
}
