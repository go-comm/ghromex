package engine_test

import (
	"testing"

	"github.com/go-comm/ghromex/engine"
)

// UA 默认的交互反馈（悬停/按下）：Chrome 的 UA 样式表没有 hover 规则，
// Ghromex 作为桌面 UI 框架给了一套克制的默认反馈——按钮型控件底色与
// 边框一起加深，文本类控件只加深边框。

// 按钮悬停：底色由 buttonface(#EFEFEF) 加深到 #E3E3E3。
func TestUAHoverFeedbackOnButton(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><body style="margin:0">
		<button id="b">go</button></body></html>`)
	b := doc.QuerySelector("#b")
	if svg := doc.DumpSVG(); !contains(svg, "#efefef") {
		t.Errorf("未悬停时按钮底色不是 buttonface:\n%s", svg)
	}
	hover(t, doc, b)
	if svg := doc.DumpSVG(); !contains(svg, "#e3e3e3") {
		t.Errorf("悬停时按钮底色未加深:\n%s", svg)
	}
}

// 按钮型 input（submit/reset/button）与 <button> 同款：底色随悬停加深。
func TestUAHoverFeedbackOnButtonLikeInput(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><body style="margin:0">
		<input id="s" type="submit" value="提交"></body></html>`)
	s := doc.QuerySelector("#s")
	hover(t, doc, s)
	if svg := doc.DumpSVG(); !contains(svg, "#e3e3e3") {
		t.Errorf("悬停时按钮型 input 的底色未加深:\n%s", svg)
	}
}

// type 属性的大小写不影响（属性选择器是大小写敏感的，代码分支按小写比较）。
func TestUAHoverFeedbackIgnoresTypeCase(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><body style="margin:0">
		<input id="s" type="SUBMIT" value="提交"></body></html>`)
	s := doc.QuerySelector("#s")
	hover(t, doc, s)
	if svg := doc.DumpSVG(); !contains(svg, "#e3e3e3") {
		t.Errorf("type=\"SUBMIT\" 的悬停反馈未生效:\n%s", svg)
	}
}

// 文本类控件：底色保持白底，只加深边框。
func TestUAHoverFeedbackOnTextInput(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><body style="margin:0">
		<input id="t" type="text"></body></html>`)
	tin := doc.QuerySelector("#t")
	hover(t, doc, tin)
	svg := doc.DumpSVG()
	if !contains(svg, "#6a6a6a") {
		t.Errorf("悬停时文本框边框未加深:\n%s", svg)
	}
	if contains(svg, "#e3e3e3") {
		t.Errorf("文本框底色不该随悬停变（那是按钮型的行为）:\n%s", svg)
	}
}

// 按下态比悬停态更深一档。
func TestUAActiveFeedbackDeeperThanHover(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><body style="margin:0">
		<button id="b">go</button></body></html>`)
	b := doc.QuerySelector("#b")
	engine.SetActiveElement(doc, b)
	doc.Refresh()
	if svg := doc.DumpSVG(); !contains(svg, "#d4d4d4") {
		t.Errorf("按下时按钮底色未加深:\n%s", svg)
	}
}

// 勾选控件不参与悬停反馈（Chrome 也不变）。
func TestUAHoverFeedbackSkipsCheckControls(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><body style="margin:0">
		<input id="c" type="checkbox"></body></html>`)
	c := doc.QuerySelector("#c")
	hover(t, doc, c)
	if svg := doc.DumpSVG(); contains(svg, "#e3e3e3") {
		t.Errorf("勾选控件不该有按钮型的悬停反馈:\n%s", svg)
	}
}

// 作者 CSS 仍可覆盖 UA 的悬停反馈（作者来源优先于 UA 来源）。
func TestAuthorCSSOverridesUAHoverFeedback(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		button:hover { background-color: #FF0000; }
	</style></head><body style="margin:0"><button id="b">go</button></body></html>`)
	b := doc.QuerySelector("#b")
	hover(t, doc, b)
	svg := doc.DumpSVG()
	if !contains(svg, "#ff0000") {
		t.Errorf("作者 CSS 未覆盖 UA 的悬停反馈:\n%s", svg)
	}
}

// 作者的**非**悬停规则也会盖住 UA 的悬停反馈（作者来源优先，与 CSS 级联
// 一致：作者写了 button{background:blue} 就不该再有 UA 的悬停底色）。
func TestAuthorNonHoverRuleBeatsUAHover(t *testing.T) {
	doc := pseudoDoc(t, `<!doctype html><html><head><style>
		button { background-color: #0000FF; }
	</style></head><body style="margin:0"><button id="b">go</button></body></html>`)
	b := doc.QuerySelector("#b")
	hover(t, doc, b)
	svg := doc.DumpSVG()
	if !contains(svg, "#0000ff") {
		t.Errorf("作者的 button 规则未盖住 UA 的悬停反馈:\n%s", svg)
	}
}
