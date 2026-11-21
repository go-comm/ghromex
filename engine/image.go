package engine

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/draw"
	_ "image/gif"  // 注册 GIF 解码器（GIF 只取第一帧）
	_ "image/jpeg" // 注册 JPEG 解码器
	_ "image/png"  // 注册 PNG 解码器
	"math"
	"net/url"
	"strconv"
	"strings"
)

// ImageLoader 加载一张图片的原始字节；失败返回 error。
//
// src 是**已解析成绝对地址**的引用（按文档 BaseURL 解析），所以加载器拿到
// 的永远是可以直接用的地址，不用自己再拼相对路径。
//
// 加载是**异步**的：引擎在布局时登记引用，worker goroutine 调加载器并解码，
// 完成后由主循环（PendingChange / LayoutDocument）把结果收进缓存并触发
// 重排。加载器因此可能被并发调用，实现必须自行保证线程安全。
//
// 返回 error 时该图片按「空图」处理（盒子不占尺寸，页面继续渲染），错误由
// 宿主上报——引擎不打印、不弹窗。
//
// data: URL 由引擎内建解码，不会交给加载器。
type ImageLoader func(src string) ([]byte, error)

// imageEntry 是一张图片的缓存状态（images map 的值，受 htmlDocument.imgMu
// 保护）。bitmap 为 nil 表示尚未到货或加载失败——失败不重试（缓存项存在即
// 「已登记过」），否则每轮布局都会把坏地址再打一遍。
type imageEntry struct {
	bitmap *Bitmap
}

// isImageElement 判断元素是否为 <img> 替换元素。
func isImageElement(el *htmlElement) bool {
	return el != nil && strings.EqualFold(el.tagName, "img")
}

// resolveImages 在布局前把每张 <img> 的位图绑定到元素上，并登记尚未加载的
// 引用。必须跑在 layoutBox 之前：布局要按已解码的固有尺寸算盒，绘制要从
// 元素上直接取位图。
//
// 每轮全量布局都走一遍（图片可能后到、src 可能被 DOM 改掉）；已缓存的
// 图片只是 map 命中，成本与元素数同级。
func resolveImages(doc *htmlDocument) {
	if doc == nil {
		return
	}
	if base := inner(doc); base != nil {
		resolveImagesIn(base, doc)
	}
}

func resolveImagesIn(base *htmlElement, doc *htmlDocument) {
	if base == nil {
		return
	}
	if isImageElement(base) {
		bindImage(base, doc)
	}
	for _, c := range base.children {
		if cb := inner(c); cb != nil {
			resolveImagesIn(cb, doc)
		}
	}
}

// bindImage 把元素 src 解析成绝对地址，取回（或登记）对应位图。
//
// 每轮都重新取，不缓存「上次的地址」：src 被 DOM 改掉后，新地址要么命中
// 文档缓存（旧图），要么登记加载并返回 nil（盒子回到 0×0），元素上不需要
// 额外保存旧值。
func bindImage(base *htmlElement, doc *htmlDocument) {
	src := strings.TrimSpace(base.GetAttribute("src"))
	if src == "" {
		base.imgBitmap = nil
		return
	}
	base.imgBitmap = doc.imageFor(resolveURL(doc.baseURL, src))
}

// imageFor 返回 src 对应的已解码位图；未加载则登记加载并返回 nil。
func (doc *htmlDocument) imageFor(src string) *Bitmap {
	if src == "" {
		return nil
	}
	doc.imgMu.Lock()
	if doc.images == nil {
		doc.images = make(map[string]*imageEntry)
	}
	entry := doc.images[src]
	if entry == nil {
		entry = &imageEntry{}
		doc.images[src] = entry
		switch {
		case isDataURL(src):
			// data: 是内建的「无 I/O」来源（测试与自包含页面用），同步解码，
			// 打开文档后的首帧就能看到图，不需要等异步投递。
			entry.bitmap, _ = decodeDataURL(src)
		case doc.loadImage != nil:
			go doc.fetchImage(src)
		default:
			// 没有加载器：按空图处理（盒子 0×0），不打印也不重试。
		}
	}
	bitmap := entry.bitmap
	doc.imgMu.Unlock()
	return bitmap
}

// fetchImage 在 worker goroutine 里读字节并解码，最后把位图写回缓存并给
// imgReady 记一笔（主 goroutine 下次 drainImages 时换算成 changed）。
//
// worker **不碰渲染状态**：只写 imgMu 保护的缓存项与原子计数。用「互斥锁 +
// 原子计数」而不是向 channel 投递结果——文档被宿主替换却忘了 Close 时，
// 投递式的 worker 会永远阻塞在发送上，顺带把整份文档（以及已解码的位图）
// 钉在内存里；写回式没有这个收尾问题，worker 加载完就自然退出。
func (doc *htmlDocument) fetchImage(src string) {
	data, err := doc.loadImage(src)
	var bitmap *Bitmap
	if err == nil {
		bitmap, err = decodeImageBytes(data)
	}
	if err != nil {
		bitmap = nil // 失败按空图处理
	}
	doc.imgMu.Lock()
	if entry := doc.images[src]; entry != nil {
		entry.bitmap = bitmap
	}
	doc.imgMu.Unlock()
	doc.imgReady.Add(1)
}

// drainImages 把「新到货的图片数」换算成一次重排级变更（主循环据此触发
// 重排）。非阻塞：没有新结果时立即返回。
//
// 只由主 goroutine 调用（PendingChange 与 LayoutDocument）。这里**不**调
// onChanged：LayoutDocument 开头也会 drain，回调里重入布局会踩坏本轮状态；
// 主循环本来就每轮比对 ChangeCount，计数增长足够唤醒重排。
func (doc *htmlDocument) drainImages() {
	if doc == nil {
		return
	}
	if n := doc.imgReady.Swap(0); n > 0 {
		doc.changed += int64(n)
	}
}

// isDataURL 判断 src 是否为 data: URL（大小写不敏感）。
func isDataURL(src string) bool {
	return len(src) >= 5 && strings.EqualFold(src[:5], "data:")
}

// decodeDataURL 解码 data:[<mediatype>][;base64],<data>。
//
// 只支持 base64 与百分号转义两种载荷；媒体类型不校验——真正的格式由
// image.Decode 判断，写错 mediatype 仍能解码（与浏览器宽容口径一致）。
func decodeDataURL(src string) (*Bitmap, error) {
	rest := src[5:]
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return nil, errors.New("engine: malformed data URL")
	}
	meta, payload := rest[:comma], rest[comma+1:]
	var data []byte
	var err error
	if strings.Contains(strings.ToLower(meta), ";base64") {
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			data, err = base64.RawStdEncoding.DecodeString(payload)
		}
	} else {
		var text string
		text, err = url.PathUnescape(payload)
		if err == nil {
			data = []byte(text)
		}
	}
	if err != nil {
		return nil, err
	}
	return decodeImageBytes(data)
}

// decodeImageBytes 解码 PNG / JPEG / GIF（GIF 取第一帧）并转成 RGBA8888。
//
// 转换目标选 NRGBA 而不是 RGBA：NRGBA 是**非预乘** alpha，与 Bitmap 的
// 像素约定一致；画到 RGBA 会把颜色预乘，半透明像素再按直通公式混合就会
// 偏暗。
func decodeImageBytes(data []byte) (*Bitmap, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, errors.New("engine: empty image")
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Src)
	return &Bitmap{W: w, H: h, Pix: dst.Pix}, nil
}

// imageDeclaredBox 给出 <img> 在布局阶段应使用的声明尺寸（内容盒，未扣
// padding/border）；-1 表示未知（尚未解码且没有声明）。
//
// 口径对齐 CSS 2.1 §10.3.2 / §10.4 的替换元素尺寸：
//   - CSS width/height 非 auto 时优先（百分比按包含块解析）；
//   - 其次 HTML width/height 属性（presentational hint，等价作者写了尺寸）；
//   - 再次按固有宽高比从已知的一边推另一边；
//   - 最后用固有像素尺寸。
//
// 两边都未知时返回 -1：未加载的 <img> 占 0×0，解码到货后重排（与浏览器
// 先按属性占位、图片到货再定尺的观感一致）。
func imageDeclaredBox(comp CSSStyleDeclaration, el *htmlElement, pctW, pctH int) (w, h int) {
	w, h = -1, -1
	if comp != nil {
		if s := comp.Width(); s != nil && s.Type() != SIZE_AUTO {
			w = resolveLen(s, pctW)
		}
		if s := comp.Height(); s != nil && s.Type() != SIZE_AUTO {
			// 百分比高度在包含块高度不确定时按 auto 处理（CSS 2.1 §10.5）。
			if s.Type() != SIZE_PERCENT || pctH >= 0 {
				h = resolveLen(s, pctH)
			}
		}
	}

	natW, natH := 0, 0
	if el != nil && el.imgBitmap != nil {
		natW, natH = el.imgBitmap.W, el.imgBitmap.H
	}
	attrW, hasW := imageAttrLen(el, "width")
	attrH, hasH := imageAttrLen(el, "height")

	ratio := 0.0
	if natW > 0 && natH > 0 {
		ratio = float64(natW) / float64(natH)
	}

	// 先落属性能直接给出的边，再按纵横比补另一边（否则 height 属性
	// 先于 h 赋值参与推导，会把 #c 算成 4×8 而不是 16×8）。
	if w < 0 && hasW {
		w = attrW
	}
	if h < 0 && hasH {
		h = attrH
	}
	if w < 0 {
		switch {
		case h >= 0 && ratio > 0:
			w = int(math.Round(float64(h) * ratio))
		case natW > 0:
			w = natW
		}
	}
	if h < 0 {
		switch {
		case w >= 0 && ratio > 0:
			h = int(math.Round(float64(w) / ratio))
		case natH > 0:
			h = natH
		}
	}
	return w, h
}

// imageAttrLen 解析 HTML 的 width/height 属性：非负整数像素，容忍 "px"
// 后缀与首尾空白；非法值按「未声明」处理（ok=false）。
func imageAttrLen(el *htmlElement, name string) (int, bool) {
	if el == nil {
		return 0, false
	}
	value := strings.TrimSpace(el.GetAttribute(name))
	if value == "" {
		return 0, false
	}
	value = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(value), "px"))
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
