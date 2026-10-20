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
//   - 行高取该行最高单元格（各单元格自身高度暂不拉伸，Chrome 是等高 +
//     垂直居中，属 v1 边界）；
//   - caption 作为表格上方的居中块。
//
// v1 边界：不支持 colspan/rowspan、不支持 border-collapse、不支持表格嵌套
// 的独立列宽计算（嵌套表格按内容宽参与）。
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

// layoutTable 布局表格：列宽对齐 + 行堆叠，返回表格外盒高度。
// 表��内容不参与常规流（由本函数接管），故 thead/tbody/tr/td 不会重复排布。
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
	// 列起点（列间 2px 间隙，近似 Chrome 的 border-spacing）
	colX := make([]int, len(colW))
	total := 0
	for i, w := range colW {
		colX[i] = total
		total += w + 2
	}
	if total > 0 {
		total -= 2 // 末列后无间隙
	}

	// caption：表格上方居中（宽度=表格宽）
	y := cy
	if cap := tableCaption(tbl); cap != nil {
		if cap.computed != nil {
			cap.computed.SetWidth(NewSize(SIZE_PIXEL, maxZero(total-2), 0))
		}
		layoutBox(g, cap, total, 0, cx, y, total, -1)
		y += cap.layoutH
	}

	// 第二遍：按列宽与行高落位（单元格的计算宽度锁定为列宽的内容宽）
	for _, row := range rows {
		cells := tableRowCells(row)
		rowH := 0
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
		y += rowH
	}

	// 表格盒：宽度按内容收缩（Chrome 同），高度为 caption + 各行
	tbl.setWidth(total)
	tbl.setHeight(y - cy)
	return y - cy
}
