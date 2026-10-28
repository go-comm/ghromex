package engine

// 变更档位与两个变更计数的统一读口。
//
// # 为什么要有档位
//
// 主循环靠 ChangeCount 判断「要不要布局」，于是**任何**变更——包括只改颜色
// 的 :hover——都会升级成整棵树的级联 + 布局。42 万元素的压力页上鼠标随便
// 滑一下就是 1.7 秒卡死（见 README「大文档滚动」），而那次重排里盒模型一个
// 像素都不会变。
//
// 档位把「重排 + 重绘」与「只重绘」分开：
//
//	ChangeReflow     盒模型可能变了 → LayoutDocument（级联 + 布局）+ 重绘
//	ChangePaintOnly  样式已就地重算、盒模型没变 → 跳过布局直接重绘
//	ChangeNone       没有待处理变更
//
// # 为什么是两个计数而不是改 ChangeCount 的返回值
//
// ChangeCount 是公开 API（宿主自己实现 HTMLDocument 时也要有），把它改成
// (count, level) 元组会破坏所有调用方与实现方；而「脏布尔」（LayoutDirty/
// PaintDirty）丢信息——读一次就没了，且「重排与重绘同时待处理」时两个布尔
// 无法表达优先级。两个**各自单调的计数** + 调用方留存基线（与主循环的
// lastChange 同款）既不破坏既有 API，又能组合出任何档位判断。
//
// # 计数的分工
//
//	ChangeCount()  重排级计数：markChanged/markScrollChanged 时 +1，
//	               LayoutDocument 收尾把它记平（lastLayoutChange）。滚动
//	               快路径的判据「changed 与 scrollMark 增量相等」只看它，
//	               纯绘制变更**不进这本账**，所以悬停不会污染滚动快路径。
//	PaintCount()   重绘级计数：markPaintChanged 时 +1，主循环重绘后记平。
//	               这类变更的样式在**标记前**就已就地重算完（见
//	               applyStateChange），布局无事可做，只是提醒重绘一帧。

// ChangeLevel 是一次变更需要的处理档位（见包注释）。
type ChangeLevel uint8

const (
	// ChangeNone：自基线以来没有待处理变更。
	ChangeNone ChangeLevel = iota
	// ChangePaintOnly：只影响绘制（颜色/背景/边框色那类），盒模型没变。
	// 样式已在状态变更时就地重算，主循环跳过 LayoutDocument 直接重绘。
	ChangePaintOnly
	// ChangeReflow：盒模型可能变了，必须 LayoutDocument 全量级联 + 布局。
	ChangeReflow
)

// PendingChange 返回自调用方基线 (reflowBase, paintBase) 以来待处理变更的
// 档位，以及两个计数的当前值（即处理完这一档后应记下的新基线）。
//
// 用法（主循环的布局门）：
//
//	level, reflow, paint := engine.PendingChange(doc, lastChange, lastPaint)
//	switch level {
//	case engine.ChangeReflow:
//	    engine.LayoutDocument(doc)
//	    lastChange, lastPaint = reflow, paint
//	case engine.ChangePaintOnly:
//	    lastPaint = paint // 只重绘，不布局
//	}
//
// 档位取两本账里的**最高**档：重排待处理时无视重绘计数（全量布局会顺带
// 消化掉样式），全部记平即 ChangeNone。
//
// 非引擎实现的文档（宿主自定义 HTMLDocument）没有重绘级账本——其状态类
// 变更本来就走不进引擎的 markPaintChanged——此时 paint 原样返回
// paintBase，档位只由 ChangeCount 决定。
func PendingChange(doc HTMLDocument, reflowBase, paintBase int64) (ChangeLevel, int64, int64) {
	if d, ok := doc.(*htmlDocument); ok {
		reflow, paint := d.changed, d.paintChanged
		switch {
		case reflow != reflowBase:
			return ChangeReflow, reflow, paint
		case paint != paintBase:
			return ChangePaintOnly, reflow, paint
		default:
			return ChangeNone, reflow, paint
		}
	}
	reflow := doc.ChangeCount()
	if reflow != reflowBase {
		return ChangeReflow, reflow, paintBase
	}
	return ChangeNone, reflow, paintBase
}

// PaintCount 返回重绘级变更计数（与 ChangeCount 配对的第二个单调基线，
// 见包注释）。主循环用它与上次记下的基线比对，判断「只有样式变了、跳过
// 布局直接重绘」。
func PaintCount(doc HTMLDocument) int64 {
	if d, ok := doc.(*htmlDocument); ok {
		return d.paintChanged
	}
	return 0
}
