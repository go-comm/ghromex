package engine

// 布局期的缓冲复用（阶段一）。
//
// 全量重排的成本几乎全在「每轮从头重建 runs」：19.5MB 的 credits 页（780 个
// `<pre>`）稳定态一次重排实测 **537ms / 1.76GB 分配**，其中 94.7% 的分配来自
// `layoutChildren` 里往 `node.runs` 追加 `textRun`——每 token 一项，全页约
// 450 万项、56B/项 ≈ 250MB，而切片每轮从 nil 重新 grow（Go 对大切片按 1.25×
// 扩容，累计分配约 5 倍终值），加上 GC 要扫描这 250MB 指针，一次重排 60% 的
// CPU 花在 `runtime.scanObject` 上。
//
// runs 是**布局结果**，该占的内存必须占；不该占的是「每轮重新分配一遍」。
// 这里让缓冲跨轮复用：内容照旧每轮重写，只有底层数组留下。实测把单次重排的
// 分配从 1.76GB 降到 36MB。
//
// 行盒累加缓冲（`curLine`）同理：长 `<pre>` 不软换行，一行可能几十万项，
// 每轮从零 grow 又是几百 MB，故一并挂在元素上复用，但超过 `maxLineScratch`
// 的缓冲不保留（换来的常驻内存比省下的时间贵）。

// layoutCache 是元素级的布局缓冲缓存（懒分配）：普通元素恒为 nil，只有
// 行内内容多的容器才会建立。
type layoutCache struct {
	// lineScratch 是 layoutChildren 的行盒累加缓冲（curLine）。flushLine
	// 只把它截空、不释放底层数组，于是下一轮直接接着用。
	lineScratch []lineItem
}

// maxLineScratch 是行盒缓冲的保留上限（项数）。一行放不下这么多才值得留：
// 常见段落一行十几项，4096 项已经宽裕；超长不折行的行（white-space:pre /
// nowrap）不缓存——实测预拿满 64k 项只换到 <10% 的时间，却要按每个容器
// 64k×120B ≈ 8MB 常驻，一页几十个这样的容器就要几百 MB。
const maxLineScratch = 4096

// takeLineBuf 取本轮的行盒累加缓冲：上一轮留的就复用，否则交给 append 现建。
// 取走即与缓存脱钩（置 nil）——万一布局过程中重入（onChanged→宿主刷新，
// textarea 折行缓存就为这个改过写法），两次布局不会共用同一块底层数组。
func (el *htmlElement) takeLineBuf() []lineItem {
	if el.lcache == nil {
		return nil
	}
	buf := el.lcache.lineScratch
	el.lcache.lineScratch = nil
	return buf[:0]
}

// putLineBuf 归还行盒累加缓冲。项数超上限的不留（宁可在下轮重新 grow，
// 也不让一行的峰值变成整个生命的常驻）；留下的先清空各项，免得缓冲里
// 挂着上一轮的 token 文本——文档一改那些旧串就再也用不到了。
func (el *htmlElement) putLineBuf(buf []lineItem) {
	if cap(buf) > maxLineScratch {
		return
	}
	for i := range buf {
		buf[i] = lineItem{}
	}
	if el.lcache == nil {
		el.lcache = &layoutCache{}
	}
	el.lcache.lineScratch = buf[:0]
}

// trimRuns 释放 runs 缓冲尾部指向旧文本的引用：缓冲跨轮复用后，容量大于
// 新长度的那一段仍挂着上一轮的 textRun（其中 text 是旧文档内容的子串，
// 会把整篇旧文本钉在内存里）。缩得不多的场合不值得再走一遍尾部，直接放过。
func trimRuns(b *htmlElement) {
	n := len(b.runs)
	if n == 0 || n*4 >= cap(b.runs) {
		return
	}
	for i := n; i < cap(b.runs); i++ {
		b.runs[i] = textRun{}
	}
}
