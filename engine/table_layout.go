package engine

import (
	"strings"
)

// ---------------------------------------------------------------------------
// 表格布局（最小可用模型）
//
// 引擎此前没有表格模型：table/thead/tbody/tr/td 都是默认的 inline-block，
// 单元格全部挤在一行（与 Chrome 的"按行成行、列对齐"差别很大）。
// 这里实现最小可用版本：
//
//   - 行：table 下的 thead/tbody/tfoot 中的 tr（以及 table 的直接 tr）；
//   - 列宽：取各行该列单元格的最大外宽（Chrome 同口径）；
//   - 单元格间留 2px 间隙（近似 Chrome 的 border-spacing: 2px）；
//     `border-collapse: collapse` 时改为相邻无缝、单元格只画上/左边框，
//     外缘右/下由表格补画（见 collapseCellBorders）；
//   - 行高取该行最高单元格（各单元格自身高度暂不拉伸，Chrome 是等高 +
//     垂直居中，属 v1 边界）；
//   - caption 作为表格上方的居中块。
//
// v1 边界：不支持 colspan/rowspan、collapse 下不做「宽者胜」的边框冲突
// 仲裁（相邻边框取后者）、不支持表格嵌套的独立列宽计算（嵌套表格按内容宽参与）。
// ---------------------------------------------------------------------------

// tableRows 返回表格的行（文档顺序）。
func tableRows(tbl *htmlElement) []*htmlElement {
	if tbl == nil {
		return nil
	}
	var rows []*htmlElement
	for _, c := range tbl.children {
		cb := inner(c)
		if cb == nil {
			continue
		}
		switch strings.ToLower(cb.tagName) {
		case "tr":
			rows = append(rows, cb)
		case "thead", "tbody", "tfoot":
			for _, rc := range cb.children {
				rb := inner(rc)
				if rb != nil && strings.EqualFold(rb.tagName, "tr") {
					rows = append(rows, rb)
				}
			}
		}
	}
	return rows
}

// tableRowCells 返回一行的单元格（td/th）。
func tableRowCells(row *htmlElement) []*htmlElement {
	if row == nil {
		return nil
	}
	var cells []*htmlElement
	for _, c := range row.children {
		cb := inner(c)
		if cb == nil {
			continue
		}
		switch strings.ToLower(cb.tagName) {
		case "td", "th":
			cells = append(cells, cb)
		}
	}
	return cells
}

// tableCaption 返回表格的 caption 元素（无则 nil）。
func tableCaption(tbl *htmlElement) *htmlElement {
	if tbl == nil {
		return nil
	}
	for _, c := range tbl.children {
		cb := inner(c)
		if cb != nil && strings.EqualFold(cb.tagName, "caption") {
			return cb
		}
	}
	return nil
}

// tableCellOuterW 返回单元格外宽（内容 + padding + border）。
func tableCellOuterW(cell *htmlElement) int {
	if cell == nil {
		return 0
	}
	pd, bd := resolvedEdges(cell, cell.computed)
	return cell.width + pd[eLeft] + pd[eRight] + bd[eLeft] + bd[eRight]
}

// collapseCellBorders 把单元格边框改成「塌陷」归属：只保留上/左两条
// （相邻单元格共享的那条线由右边邻居的左边框、下一行的上边框承担），
// 外缘的右/下两条由表格自己补画（见 layoutTable 末尾）。
//
// 这是浏览器「半边框」模型在整数像素下的等价形式：Chrome 实测
// `border-collapse: collapse` + `border: 1px solid` + `padding: 4px 8px`
// 的单元格，外盒 = 内容 + 16 + 1（左右各摊 0.5px），行高 = 内容 + 8 + 1。
// 若照 separate 那样四边都画，单元格会宽/高各多 1px，相邻两格之间还会出现
// 2px 双线。
func collapseCellBorders(cell *htmlElement) {
	if cell == nil || cell.computed == nil {
		return
	}
	bd := resolveEdgeRect(cell.computed.BorderStyleWidth(), cell.width)
	zero := NewZeroSize()
	cell.computed.SetBorderStyleWidth(NewRect(
		NewSize(SIZE_PIXEL, bd[eLeft], 0), // left
		zero,                              // right
		NewSize(SIZE_PIXEL, bd[eTop], 0),  // top
		zero /* bottom */))
}

// collapsedOuterBorder 返回塌陷表格外缘边框的宽度与颜色：取第一个带边框
// 单元格的上边框（collapse 下四边通常等宽同色，相邻线共享）。表格自身已声明
// 边框时不调用——作者样式优先。
func collapsedOuterBorder(rows []*htmlElement) (int, Color) {
	for _, row := range rows {
		for _, cell := range tableRowCells(row) {
			if cell == nil || cell.computed == nil {
				continue
			}
			bd := resolveEdgeRect(cell.computed.BorderStyleWidth(), cell.width)
			if bd[eTop] > 0 {
				return bd[eTop], cell.computed.BorderColor()
			}
		}
	}
	return 0, nil
}

// layoutTable 布局表格：列宽对齐 + 行堆叠，返回表格外盒高度。
// 表格内容不参与常规流（由本函数接管），故 thead/tbody/tr/td 不会重复排布。
func layoutTable(g Graphics, tbl *htmlElement, contentW, cx, cy int) int {
	rows := tableRows(tbl)
	if len(rows) == 0 {
		// 空表格：只排 caption
		if cap := tableCaption(tbl); cap != nil {
			layoutBox(g, cap, contentW, 0, cx, cy, contentW, -1)
			return cap.layoutH
		}
		return 0
	}

	// border-collapse: collapse —— 相邻单元格共享边框（语义见 BorderCollapse）。
	// 必须在量列宽之前改单元格边框：外盒尺寸取决于它（每边只摊 0.5px，整数
	// 像素下等价于「只留上/左」）。
	collapse := tbl.computed != nil && tbl.computed.BorderCollapse() == BorderCollapseCollapse
	gap := 2 // 单元格间隙，近似 Chrome 的 border-spacing: 2px
	outerW := 0
	var outerColor Color
	if collapse {
		gap = 0
		for _, row := range rows {
			for _, cell := range tableRowCells(row) {
				collapseCellBorders(cell)
			}
		}
		outerW, outerColor = collapsedOuterBorder(rows)
	}

	// 第一遍：各单元格按内容自然宽度布局，量出列宽
	colW := []int{}
	for _, row := range rows {
		for i, cell := range tableRowCells(row) {
			layoutBox(g, cell, contentW, 0, cx, cy, contentW, -1)
			w := tableCellOuterW(cell)
			for len(colW) <= i {
				colW = append(colW, 0)
			}
			if w > colW[i] {
				colW[i] = w
			}
		}
	}
	// 列起点（separate 下留 2px 间隙，近似 Chrome 的 border-spacing；
	// collapse 下相邻无缝）
	colX := make([]int, len(colW))
	total := 0
	for i, w := range colW {
		colX[i] = total
		total += w + gap
	}
	if total > 0 {
		total -= gap // 末列后无间隙
	}

	// caption：表格上方居中（宽度=表格盒宽；collapse 下含表格外缘边框）
	y := cy
	if cap := tableCaption(tbl); cap != nil {
		capW := maxZero(total - 2)
		if collapse {
			capW = total + outerW
		}
		if cap.computed != nil {
			cap.computed.SetWidth(NewSize(SIZE_PIXEL, capW, 0))
		}
		layoutBox(g, cap, capW, 0, cx, y, capW, -1)
		y += cap.layoutH
	}

	// 第二遍：按列宽与行高落位（单元格的计算宽度锁定为列宽的内容宽）
	for _, row := range rows {
		cells := tableRowCells(row)
		rowH := 0
		rowX := cx
		if len(colX) > 0 {
			rowX = cx + colX[0]
		}
		for i, cell := range cells {
			if i >= len(colW) {
				break
			}
			if cell.computed != nil {
				pd, bd := resolvedEdges(cell, cell.computed)
				inner := colW[i] - pd[eLeft] - pd[eRight] - bd[eLeft] - bd[eRight]
				cell.computed.SetWidth(NewSize(SIZE_PIXEL, maxZero(inner), 0))
			}
			layoutBox(g, cell, colW[i], 0, cx+colX[i], y, colW[i], -1)
			if cell.layoutH > rowH {
				rowH = cell.layoutH
			}
		}
		// 行盒也要有几何（浏览器的 getBoundingClientRect 对 tr 返回行盒）：
		// 否则行/行组的矩形恒为 (0,0,0,0)。
		row.setX(rowX)
		row.setY(y)
		row.setWidth(total)
		row.setHeight(rowH)
		y += rowH
	}

	// 行组（thead/tbody/tfoot）：取其各行的并集作为自身盒。
	for _, c := range tbl.children {
		cb := inner(c)
		if cb == nil {
			continue
		}
		switch strings.ToLower(cb.tagName) {
		case "thead", "tbody", "tfoot":
			gx, gy, gw, gh := 0, 0, 0, 0
			first := true
			for _, rc := range cb.children {
				rb := inner(rc)
				if rb == nil || !strings.EqualFold(rb.tagName, "tr") {
					continue
				}
				rx, ry := rb.X(), rb.Y()
				rw, rh := rb.Width(), rb.Height()
				if first {
					gx, gy, gw, gh = rx, ry, rw, rh
					first = false
					continue
				}
				if rx < gx {
					gx = rx
				}
				if ry < gy {
					gy = ry
				}
				if rx+rw > gx+gw {
					gw = rx + rw - gx
				}
				if ry+rh > gy+gh {
					gh = ry + rh - gy
				}
			}
			if !first {
				cb.setX(gx)
				cb.setY(gy)
				cb.setWidth(gw)
				cb.setHeight(gh)
			}
		}
	}

	// 表格盒：宽度按内容收缩（Chrome 同），高度为 caption + 各行
	tbl.setX(cx)
	tbl.setY(cy)
	tbl.setWidth(total)
	tbl.setHeight(y - cy)
	boxH := y - cy
	if collapse && outerW > 0 && tbl.computed != nil {
		// 塌陷表格外缘的右/下两条边归表格（Chrome 的 collapse 表边框盒 =
		// 单元格盒之和 + 外缘半个边框）。仅在本表自身未声明边框时补画，
		// 作者样式优先。补画后盒子宽/高各多 1px，流内推进按盒子算。
		own := resolveEdgeRect(tbl.computed.BorderStyleWidth(), tbl.width)
		if own[eTop] == 0 && own[eRight] == 0 && own[eBottom] == 0 && own[eLeft] == 0 {
			zero := NewZeroSize()
			tbl.computed.SetBorderStyleWidth(NewRect(
				zero,                           // left
				NewSize(SIZE_PIXEL, outerW, 0), // right
				zero,                           // top
				NewSize(SIZE_PIXEL, outerW, 0) /* bottom */))
			if outerColor != nil {
				tbl.computed.SetBorderColor(outerColor)
			}
			tbl.computed.SetBorderStyle(BorderStyleSolid)
			boxH += outerW
		}
	}
	return boxH
}
