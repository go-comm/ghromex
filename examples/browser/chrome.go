// 本文件是浏览器外壳（工具条 + 状态栏）的样式表；包文档见 main.go。
//
// 引擎没有窗口 chrome 层，外壳只能作为文档的一部分注入到每一份加载的
// 文档里，样式也因此用类选择器而不是内联 style——类名统一带 gh- 前缀，
// 避免与页面作者自己的类名冲突。
//
// 用到的引擎 CSS 子集要点：
//   - 不支持单边 border（border-bottom 之类），分隔线用整圈 border 或
//     单独的细条元素；
//   - 不支持 flex/grid，工具条里的控件一律绝对定位；
//   - 不支持 box-shadow，层次靠纯色与 border-radius 拉开；
//   - 按钮文字按内容顶部排（此前有垂直居中的问题，已在引擎侧修过），
//     这里用固定高度 + text-align 让它居中。
package main

const chromeCSS = `
.gh-bar {
	position: fixed;
	left: 0;
	top: 0;
	width: 100%;
	height: 44px;
	background-color: #0F172A;
	z-index: 1000;
}
.gh-btn {
	position: absolute;
	top: 6px;
	width: 40px;
	height: 32px;
	padding: 0;
	background-color: #1E293B;
	border: 1px solid #334155;
	border-radius: 8px;
	/* 图标（← → ↻ ⌂ 是字形，不是图片——引擎还不支持 <img>）用纯白：
	   深色工具条上浅灰蓝的字形发灰、不醒目，一眼看不出是按钮。 */
	color: #FFFFFF;
	font-size: 16px;
	text-align: center;
	box-sizing: border-box;
	z-index: 1001;
}
.gh-btn-off {
	/* 禁用态要"暗"（一眼看出点不了），但不能暗到看不见字形。 */
	background-color: #16213A;
	border-color: #233047;
	color: #64748B;
}
.gh-addr {
	position: absolute;
	top: 7px;
	height: 30px;
	padding: 0 12px;
	background-color: #FFFFFF;
	border: 1px solid #CBD5E1;
	border-radius: 8px;
	color: #0F172A;
	font-size: 13px;
	box-sizing: border-box;
	z-index: 1001;
}
.gh-go {
	position: absolute;
	top: 6px;
	height: 32px;
	padding: 0;
	/* 不用蓝色：深色工具条上唯一一处高饱和色，与整条工具条不搭（页面下方
	   的蓝色点缀反而把它衬得更突兀）。改成与导航按钮同族的深板岩 + 白字，
	   边框比导航按钮亮一档，位置与体量靠右缘（引擎没有 margin:auto，
	   位置由程序按视口宽度算）自然区分。 */
	background-color: #1E293B;
	border: 1px solid #64748B;
	border-radius: 8px;
	color: #FFFFFF;
	font-size: 13px;
	text-align: center;
	box-sizing: border-box;
	z-index: 1001;
}
.gh-status {
	position: fixed;
	left: 0;
	bottom: 0;
	width: 100%;
	height: 26px;
	background-color: #F8FAFC;
	z-index: 1000;
}
.gh-status-title {
	position: absolute;
	left: 12px;
	top: 5px;
	color: #334155;
	font-size: 12px;
}
.gh-status-hint {
	position: absolute;
	right: 12px;
	top: 5px;
	color: #94A3B8;
	font-size: 12px;
}
.gh-spacer-top {
	height: 44px;
}
.gh-spacer-bottom {
	height: 26px;
}
`
