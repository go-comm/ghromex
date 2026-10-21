package sdl2

import (
	"encoding/binary"
	"os"
	"sync"
)

// 字体行度量（hhea 表）——行盒高度的口径来源。
//
// 浏览器（Chrome/Windows 走 DirectWrite）的 line-height:normal 取字体的
// 行度量 ascent+descent+lineGap，并按「行距一半在基线上、一半在基线下」
// 分配。实测（Chrome headless 量得）与本文件的解析口径一致：
//
//	ascent  = round(hhea.ascender  * size / unitsPerEm)
//	descent = round(hhea.descender * size / unitsPerEm)
//	          + round(hhea.lineGap  * size / unitsPerEm)
//
// 即行距（lineGap）整体计入下降部。抽样核对（Arial/Times/雅黑/Segoe UI/
// Courier New/Impact @16px）：14+4=18、14+4=18、17+4=21、17+4=21、13+5=18、
// 16+3=19，与 Chrome 实测行高逐项相同。
//
// 不用 SDL_ttf 的 TTF_FontAscent/Descent：那两个数是 FreeType 的取整口径
// （Arial 15/-3、雅黑 17/-4、SimHei 14/-2…），与浏览器的取整不同（雅黑 21、
// SimHei 18），行高会逐行偏差并累积成整页错位。

// ttfMetrics 是字体文件里的 hhea 行度量（字体单位，未缩放）。
type ttfMetrics struct {
	upem                     int
	ascent, descent, lineGap int
}

var (
	ttfMetricsMu    sync.Mutex
	ttfMetricsCache = map[string]ttfMetrics{}
)

// loadTTFMetrics 解析字体文件的 head（unitsPerEm）与 hhea（行度量）表。
// 支持 .ttf/.otf（单字体）与 .ttc/.otc（字体集合，取第 0 号 face）。
// 文件缺失或表结构异常时返回 false——调用方退回估算值，不让渲染中断。
func loadTTFMetrics(path string) (ttfMetrics, bool) {
	ttfMetricsMu.Lock()
	if m, ok := ttfMetricsCache[path]; ok {
		ttfMetricsMu.Unlock()
		return m, true
	}
	ttfMetricsMu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil || len(data) < 12 {
		return ttfMetrics{}, false
	}
	base := 0
	if string(data[0:4]) == "ttcf" {
		// 字体集合：偏移表给出各 face 的起点，取第 0 号。
		if len(data) < 16 {
			return ttfMetrics{}, false
		}
		base = int(binary.BigEndian.Uint32(data[12:16]))
		if base < 0 || base+12 > len(data) {
			return ttfMetrics{}, false
		}
	}
	numTables := int(binary.BigEndian.Uint16(data[base+4 : base+6]))
	tables := map[string][]byte{}
	for i := 0; i < numTables; i++ {
		off := base + 12 + i*16
		if off+16 > len(data) {
			break
		}
		tag := string(data[off : off+4])
		toff := int(binary.BigEndian.Uint32(data[off+8 : off+12]))
		tlen := int(binary.BigEndian.Uint32(data[off+12 : off+16]))
		if toff < 0 || tlen < 0 || toff+tlen > len(data) {
			continue
		}
		tables[tag] = data[toff : toff+tlen]
	}
	head, hhea, os2 := tables["head"], tables["hhea"], tables["OS/2"]
	if len(head) < 54 || len(hhea) < 36 {
		return ttfMetrics{}, false
	}
	upem := int(binary.BigEndian.Uint16(head[18:20]))
	if upem <= 0 {
		return ttfMetrics{}, false
	}
	m := ttfMetrics{
		upem:    upem,
		ascent:  int(int16(binary.BigEndian.Uint16(hhea[4:6]))),
		descent: -int(int16(binary.BigEndian.Uint16(hhea[6:8]))),
		lineGap: int(int16(binary.BigEndian.Uint16(hhea[8:10]))),
	}
	// Chrome/Windows 走 DirectWrite，行高取 OS/2 的 usWinAscent/usWinDescent
	// （未置 USE_TYPO_METRICS 位时），而不是 hhea。两者多数系统字体相同，
	// 但 Noto Sans SC 这类字体差别很大（hhea 1000/200 仅为 19@16px，
	// win 1160/288 才是 Chrome 实测的 19/5 = 24px）。
	if len(os2) >= 78 {
		fsSel := int(binary.BigEndian.Uint16(os2[62:64]))
		if fsSel&0x80 != 0 { // USE_TYPO_METRICS
			m.ascent = int(int16(binary.BigEndian.Uint16(os2[68:70])))
			m.descent = -int(int16(binary.BigEndian.Uint16(os2[70:72])))
			m.lineGap = int(int16(binary.BigEndian.Uint16(os2[72:74])))
		} else {
			winAsc := int(binary.BigEndian.Uint16(os2[74:76]))
			winDesc := int(binary.BigEndian.Uint16(os2[76:78]))
			if winAsc > 0 && winDesc > 0 {
				m.ascent, m.descent = winAsc, winDesc
			}
		}
	}
	ttfMetricsMu.Lock()
	ttfMetricsCache[path] = m
	ttfMetricsMu.Unlock()
	return m, true
}

// fontLineMetrics 返回（字体文件, 字号）下的行度量（像素）：ascent 为基线
// 以上高度，descent 为基线以下高度，二者之和即行盒高度。
// 文件不可解析时返回 false，调用方退回按字号的估算。
func fontLineMetrics(path string, size int) (ascent, descent int, ok bool) {
	m, ok2 := loadTTFMetrics(path)
	if !ok2 || size <= 0 {
		return 0, 0, false
	}
	s := float64(size) / float64(m.upem)
	round := func(v float64) int {
		if v < 0 {
			return -int(-v + 0.5)
		}
		return int(v + 0.5)
	}
	ascent = round(float64(m.ascent) * s)
	descent = round(float64(m.descent)*s) + round(float64(m.lineGap)*s)
	if ascent < 0 {
		ascent = 0
	}
	if descent < 0 {
		descent = 0
	}
	return ascent, descent, true
}
