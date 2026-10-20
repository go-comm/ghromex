package engine_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// ---------------------------------------------------------------------------
// 畸形输入健壮性回归
//
// 引擎要处理真实世界的任意 HTML/CSS（用户输入、剪藏片段、第三方页面）：
// 解析/级联/布局任一环节对畸形输入 panic 都是"一个坏页面搞崩整个应用"。
// 本测试把畸形输入清单固定下来，确保后续改动不引入 panic。
//
// 判据：OpenDocument 返回 error 属正常（允许拒绝），但**不得 panic**；
// 成功解析的文档还必须能导出 SVG（覆盖布局与绘制指令生成路径）。
// ---------------------------------------------------------------------------

// TestMalformedInputNoPanic 畸形 HTML/CSS/极端取值不得触发 panic。
func TestMalformedInputNoPanic(t *testing.T) {
	cases := []string{
		"<div><span>unclosed",
		"<<<<>>>>",
		"</div></p></b>",
		"<div a=1 a=2 b='x' c=\"y\" z>",
		"<!-- unterminated comment",
		"<!DOCTYPE html><html><head><style>body{color:}</style></head><body>x</body></html>",
		"<style>*{width:;color:#;margin:calc(}</style><div>x</div>",
		"<style>div{content:'unterminated</style><div>x</div>",
		"<style>@media (min-width:100px){div{color:red}}</style><div>x</div>",
		"<style>div{width:999999999px;height:999999999px}</style><div>x</div>",
		"<style>div{font-size:0px;margin:-1e9px;z-index:99999999}</style><div>x</div>",
		"<div style=\"width:calc(100% - ;color:red\">x</div>",
		"<input type=checkbox checked><input type=radio checked><select><option>a</select>",
		"<textarea></textarea><textarea>unclosed",
		"<form><form><input name=a></form></form>",
		"<table><tr><td>a<td>b</table>",
		"<p>" + strings.Repeat("&amp;", 100) + "</p>",
		"<div>\x00\x01 control chars</div>",
		"<div>bad utf8 \xff\xfe</div>",
		"<div>" + strings.Repeat("中", 5000) + "</div>",
		"<a href='#'>x</a><a href=\"javascript:alert(1)\">y</a>",
		"<div style=\"background:url(javascript:alert(1))\">z</div>",
	}
	for i, src := range cases {
		src := src
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic: %v\nsrc=%q", r, src)
				}
			}()
			vp := engine.NewHeadlessViewport(200, 100)
			doc, err := engine.OpenDocument(vp, src)
			if err != nil {
				return // 允许拒绝解析，但不能崩
			}
			_ = doc.DumpSVG() // 布局 + 绘制指令路径也要走通
		})
	}
}

// TestDeepNestingSurvives 深层嵌套不得栈溢出。栈溢出是 fatal error
// （recover 抓不到），故单独成测：一旦回归，本测试会以进程崩溃暴露。
func TestDeepNestingSurvives(t *testing.T) {
	for _, depth := range []int{100, 1000, 5000} {
		src := strings.Repeat("<div>", depth) + "x" + strings.Repeat("</div>", depth)
		vp := engine.NewHeadlessViewport(200, 100)
		doc, err := engine.OpenDocument(vp, src)
		if err != nil {
			continue // 拒绝解析也算安全
		}
		_ = doc.DumpSVG()
	}
}
