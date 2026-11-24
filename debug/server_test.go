// 调试服务的端到端用例：用假窗口（headless 文档 + 记录注入的事件）跑完
// 整条 HTTP 链路，不需要 SDL，任意平台可跑。
//
// 假实现就地执行 Do 的任务——真实 Window 会把任务交回主循环，这里没有
// 主循环，而 headless 文档本来也只在当前 goroutine 上用，语义等价。

package debug

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
)

// fakeWin 实现 target。
type fakeWin struct {
	mu sync.Mutex

	vp        *engine.HeadlessViewport
	doc       engine.HTMLDocument
	injected  []browser.InputEvent
	opens     int
	doCalls   int
	resizeW   int
	resizeH   int
	shotCalls int
	closeReqs int
	running   bool
}

const testPage = `<!doctype html><html><head><style>
body{margin:0}
#box{position:absolute;left:100px;top:80px;width:200px;height:60px;background:#eee}
#go{position:absolute;left:120px;top:100px;width:80px;height:30px;background:#ccc}
</style></head><body>` +
	`<div id="box"><div id="go" class="btn primary" data-x="42">Go</div></div>` +
	`<div id="status">就绪</div></body></html>`

func newFakeWin(t *testing.T) *fakeWin {
	t.Helper()
	vp := engine.NewHeadlessViewport(800, 600)
	doc, err := engine.OpenDocumentWith(vp, testPage, engine.DocumentOptions{})
	if err != nil {
		t.Fatalf("打开文档: %v", err)
	}
	engine.LayoutDocument(doc)
	return &fakeWin{vp: vp, doc: doc, running: true}
}

func (f *fakeWin) Do(fn func() (any, error), _ time.Duration) (any, error) {
	f.mu.Lock()
	f.doCalls++
	running := f.running
	f.mu.Unlock()
	if !running {
		return nil, browser.ErrLoopNotRunning
	}
	// 与真实队列同口径：panic 收敛成 error，不然坏请求会把测试进程带走。
	return f.safeRun(fn)
}

func (f *fakeWin) safeRun(fn func() (any, error)) (out any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errorWithPanic(r)
		}
	}()
	return fn()
}

func errorWithPanic(r any) error {
	return fmt.Errorf("主线程任务 panic: %v", r)
}

func (f *fakeWin) Running() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running
}

func (f *fakeWin) InjectInput(ev browser.InputEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.running {
		return browser.ErrLoopNotRunning
	}
	f.injected = append(f.injected, ev)
	return nil
}

func (f *fakeWin) RequestClose() {
	f.mu.Lock()
	f.closeReqs++
	f.running = false
	f.mu.Unlock()
}

func (f *fakeWin) Document() engine.HTMLDocument {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.doc
}

func (f *fakeWin) OpenDocument(src string) (engine.HTMLDocument, error) {
	doc, err := engine.OpenDocumentWith(f.vp, src, engine.DocumentOptions{})
	if err != nil {
		return nil, err
	}
	engine.LayoutDocument(doc)
	f.mu.Lock()
	f.doc = doc
	f.opens++
	f.mu.Unlock()
	return doc, nil
}

func (f *fakeWin) ViewportWidth() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vp.W
}

func (f *fakeWin) ViewportHeight() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vp.H
}

func (f *fakeWin) Resize(w, h int) {
	f.mu.Lock()
	f.resizeW, f.resizeH = w, h
	f.vp.W, f.vp.H = w, h
	f.mu.Unlock()
}

// shotPNG 是一份固定的 1×1 PNG：只验「字节透传 + Content-Type」，
// 真实回读路径由 browser 层的截图用例负责。
var shotPNG = func() []byte {
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Pix[0], img.Pix[1], img.Pix[2], img.Pix[3] = 10, 20, 30, 255
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}()

func (f *fakeWin) ScreenshotPNG() ([]byte, error) {
	f.mu.Lock()
	f.shotCalls++
	f.mu.Unlock()
	return shotPNG, nil
}

func (f *fakeWin) events() []browser.InputEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]browser.InputEvent(nil), f.injected...)
}

// newTestServer 起一个用假窗口的调试服务，返回 httptest 记录器用的请求函数。
func newTestServer(t *testing.T, f *fakeWin) *Server {
	t.Helper()
	s, err := serve(f, Options{Addr: "127.0.0.1:0", Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatalf("起服务: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// get 发一个请求并解出 JSON（返回原始 body 供非 JSON 端点检查）。
func get(t *testing.T, s *Server, path string) (int, []byte, http.Header) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.Bytes(), rec.Header()
}

func post(t *testing.T, s *Server, path, body string) (int, []byte, http.Header) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	s.srv.Handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes(), rec.Header()
}

func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("响应不是合法 JSON: %v\n%s", err, string(body))
	}
	return m
}

// ===== 地址与安全边界 =====

func TestNormalizeAddr(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "127.0.0.1:0", false},
		{"9223", "127.0.0.1:9223", false},
		{"0", "127.0.0.1:0", false},
		{"localhost:1234", "localhost:1234", false},
		{"127.0.0.1:9223", "127.0.0.1:9223", false},
		{"[::1]:9223", "[::1]:9223", false},
		{":9223", "127.0.0.1:9223", false},
		{"0.0.0.0:9223", "", true},     // 全网卡监听：拒绝
		{"192.168.1.5:9223", "", true}, // 局域网地址：拒绝
		{"bogus", "", true},
		{"99999", "", true},
	}
	for _, c := range cases {
		got, err := normalizeAddr(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("normalizeAddr(%q) = %q, want 错误", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeAddr(%q) 报错: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("normalizeAddr(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestServeRejectsNonLoopback(t *testing.T) {
	f := newFakeWin(t)
	_, err := serve(f, Options{Addr: "0.0.0.0:9223"})
	if !errors.Is(err, ErrNotLoopback) {
		t.Errorf("serve(0.0.0.0) = %v, want ErrNotLoopback", err)
	}
}

func TestServeFromEnvDisabled(t *testing.T) {
	os.Unsetenv(EnvPort)
	s, err := ServeFromEnv(nil)
	if s != nil || err != nil {
		t.Errorf("未设环境变量时 ServeFromEnv = (%v, %v)，want (nil, nil)", s, err)
	}
	// 非法值要报错：调试开关不接受「以为开了其实没开」。
	os.Setenv(EnvPort, "not-a-port")
	defer os.Unsetenv(EnvPort)
	if s, err := ServeFromEnv(nil); err == nil || s != nil {
		t.Errorf("非法端口时 = (%v, %v)，want 报错", s, err)
	}
}

// ===== 观测端点 =====

func TestStateReportsDocument(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)
	code, body, _ := get(t, s, "/debug/state")
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d: %s", code, body)
	}
	m := decode(t, body)
	if m["ok"] != true || m["running"] != true {
		t.Errorf("ok/running = %v/%v, want true/true", m["ok"], m["running"])
	}
	vp := m["viewport"].(map[string]any)
	if int(vp["w"].(float64)) != 800 || int(vp["h"].(float64)) != 600 {
		t.Errorf("viewport = %v, want 800x600", vp)
	}
	doc := m["document"].(map[string]any)
	if doc["title"] != "" {
		t.Errorf("title = %v, want 空串（页面没标题）", doc["title"])
	}
	if _, ok := doc["changeCount"]; !ok {
		t.Error("document 里应有 changeCount（重排档计数）")
	}
}

// centerOf 读出选择器命中元素的盒中心：命中测试的坐标必须从布局里取，
// 硬编码会在「相对定位的父盒」上算错（#go 绝对定位在 #box 里，坐标不是
// CSS 里写的 left/top）。
func centerOf(t *testing.T, s *Server, selector string) (int, int) {
	t.Helper()
	_, body, _ := get(t, s, "/debug/dom?selector="+url.QueryEscape(selector))
	tree := decode(t, body)["tree"].(map[string]any)
	b := tree["box"].(map[string]any)
	x := int(b["x"].(float64)) + int(b["w"].(float64))/2
	y := int(b["y"].(float64)) + int(b["h"].(float64))/2
	return x, y
}

// TestHitFindsDeepestElement 用 /debug/dom 读出元素盒，再拿盒中心去命中——
// 不硬编码坐标，布局口径变了也不会假失败。
func TestHitFindsDeepestElement(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	_, body, _ := get(t, s, "/debug/dom?selector=%23go")
	tree := decode(t, body)["tree"].(map[string]any)
	if tree["id"] != "go" {
		t.Fatalf("dom 快照 id = %v, want go", tree["id"])
	}
	cx, cy := centerOf(t, s, "#go")

	path := fmt.Sprintf("/debug/hit?x=%d&y=%d", cx, cy)
	_, body, _ = get(t, s, path)
	hit := decode(t, body)
	chain, ok := hit["chain"].([]any)
	if !ok || len(chain) == 0 {
		t.Fatalf("命中链为空: %s", body)
	}
	first := chain[0].(map[string]any)
	if first["id"] != "go" {
		t.Errorf("链首 = %v, want #go（最深元素应在链首）", first["id"])
	}
	if first["selector"] != "#go" {
		t.Errorf("selector 猜测 = %v, want #go", first["selector"])
	}
	// 父链要带出容器：证明「命中了但被父层吃掉」这类问题看得出来。
	var sawBox bool
	for _, e := range chain {
		if m, ok := e.(map[string]any); ok && m["id"] == "box" {
			sawBox = true
		}
	}
	if !sawBox {
		t.Errorf("命中链里没有 #box: %s", body)
	}
}

func TestDOMLimits(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)
	// limit=1：只给根节点，其余标记截断。
	_, body, _ := get(t, s, "/debug/dom?limit=1")
	m := decode(t, body)
	if int(m["count"].(float64)) != 1 {
		t.Errorf("count = %v, want 1", m["count"])
	}
	if m["truncated"] != true {
		t.Errorf("truncated = %v, want true", m["truncated"])
	}
	// depth=0：不展开子树。
	_, body, _ = get(t, s, "/debug/dom?depth=0")
	m = decode(t, body)
	tree := m["tree"].(map[string]any)
	if _, has := tree["children"]; has {
		t.Errorf("depth=0 还展开了子树: %s", body)
	}
}

func TestTextAndAttrEndpoints(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	_, body, _ := get(t, s, "/debug/text?selector=%23status")
	m := decode(t, body)
	if m["text"] != "就绪" {
		t.Errorf("text = %v, want 就绪", m["text"])
	}
	_, body, _ = get(t, s, "/debug/attr?selector=%23go&name=data-x")
	m = decode(t, body)
	if m["value"] != "42" {
		t.Errorf("data-x = %v, want 42", m["value"])
	}
	// 白名单之外的属性走 /debug/attr，正好覆盖这条路径。
	_, body, _ = get(t, s, "/debug/attr?selector=%23go&name=class")
	if decode(t, body)["value"] != "btn primary" {
		t.Errorf("class = %s, want btn primary", body)
	}
	// 选择器无匹配要报错（而不是返回一个空对象让脚本以为成功）。
	code, body, _ := get(t, s, "/debug/text?selector=%23nope")
	if code != http.StatusBadRequest || !strings.Contains(string(body), "无匹配") {
		t.Errorf("无匹配选择器 = %d %s", code, body)
	}
	if code, _, _ := get(t, s, "/debug/text"); code != http.StatusBadRequest {
		t.Errorf("缺 selector = %d, want 400", code)
	}
}

func TestShotAndLayout(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	code, body, hdr := get(t, s, "/debug/shot.png")
	if code != http.StatusOK {
		t.Fatalf("shot.png = %d: %s", code, body)
	}
	if ct := hdr.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if !bytes.HasPrefix(body, []byte("\x89PNG")) {
		t.Error("shot.png 不是 PNG 字节")
	}
	if f.shotCalls != 1 {
		t.Errorf("截图调用 %d 次, want 1", f.shotCalls)
	}

	code, body, hdr = get(t, s, "/debug/layout.svg")
	if code != http.StatusOK {
		t.Fatalf("layout.svg = %d: %s", code, body)
	}
	if ct := hdr.Get("Content-Type"); !strings.HasPrefix(ct, "image/svg") {
		t.Errorf("Content-Type = %q, want image/svg+xml", ct)
	}
	if !strings.Contains(string(body), "<svg") {
		t.Error("layout.svg 里没有 <svg")
	}
}

func TestDocSwapsDocument(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	body := `{"html":"<!doctype html><html><head><title>新页</title></head><body><div id=\"hello\">世界</div></body></html>"}`
	code, out, _ := post(t, s, "/debug/doc", body)
	if code != http.StatusOK {
		t.Fatalf("POST /debug/doc = %d: %s", code, out)
	}
	if f.opens != 1 {
		t.Errorf("打开文档 %d 次, want 1", f.opens)
	}
	_, out, _ = get(t, s, "/debug/state")
	doc := decode(t, out)["document"].(map[string]any)
	if doc["title"] != "新页" {
		t.Errorf("换文档后标题 = %v, want 新页", doc["title"])
	}
	_, out, _ = get(t, s, "/debug/text?selector=%23hello")
	if decode(t, out)["text"] != "世界" {
		t.Errorf("换文档后文本 = %s, want 世界", out)
	}

	// path 走文件系统；不存在的文件要报错。
	if _, err := postCheck(t, s, "/debug/doc", `{"path":"Z:/nope/nope.html"}`); err != nil {
		t.Error(err)
	}
}

func postCheck(t *testing.T, s *Server, path, body string) (int, error) {
	t.Helper()
	code, out, _ := post(t, s, path, body)
	if code == http.StatusOK {
		return code, errors.New("期望非 200，实际读到 " + string(out))
	}
	return code, nil
}

// ===== 输入动作 =====

func TestInputClickInjectsMotionDownUp(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	code, out, _ := post(t, s, "/debug/input", `[{"op":"click","x":140,"y":110}]`)
	if code != http.StatusOK {
		t.Fatalf("POST /debug/input = %d: %s", code, out)
	}
	m := decode(t, out)
	if m["ok"] != true || int(m["executed"].(float64)) != 1 {
		t.Fatalf("响应 = %s", out)
	}
	evs := f.events()
	// 真实用户点击前光标已经在那儿：click 要补一条 motion，
	// 否则 :hover 与 CSS cursor 在按下那一刻还是上一个位置的。
	if len(evs) != 3 {
		t.Fatalf("注入 %d 个事件, want 3（motion+down+up）: %+v", len(evs), evs)
	}
	if evs[0].Kind != browser.InputMouseMove || evs[0].X != 140 || evs[0].Y != 110 {
		t.Errorf("事件 0 = %+v, want motion(140,110)", evs[0])
	}
	if evs[1].Kind != browser.InputMouseDown || evs[1].Button != browser.MouseButtonLeft {
		t.Errorf("事件 1 = %+v, want 左键按下", evs[1])
	}
	if evs[2].Kind != browser.InputMouseUp {
		t.Errorf("事件 2 = %+v, want 抬起", evs[2])
	}
}

func TestInputKeepsLastPoint(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	post(t, s, "/debug/input", `[{"op":"move","x":300,"y":200},{"op":"wheel","dy":-1}]`)
	evs := f.events()
	if len(evs) != 2 {
		t.Fatalf("注入 %d 个事件: %+v", len(evs), evs)
	}
	// 省略坐标的滚轮落在最后已知位置：脚本连着滚好几格不必每行重抄坐标。
	if evs[1].Kind != browser.InputWheel || evs[1].X != 300 || evs[1].Y != 200 || evs[1].WheelY != -1 {
		t.Errorf("滚轮事件 = %+v, want (300,200) dy=-1", evs[1])
	}
}

func TestInputKeyMapping(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	post(t, s, "/debug/input", `[
		{"op":"key","key":"ctrl+shift+s"},
		{"op":"key","key":"enter"},
		{"op":"key","key":"F5"},
		{"op":"key","key":"pagedown"},
		{"op":"key","key":"↑"}
	]`)
	evs := f.events()
	if len(evs) != 5 {
		t.Fatalf("注入 %d 个按键, want 5", len(evs))
	}
	if evs[0].Key != 's' || evs[0].Mods != browser.ModCtrl|browser.ModShift {
		t.Errorf("ctrl+shift+s = %+v", evs[0])
	}
	if evs[1].Key != browser.KeyEnter || evs[1].Mods != 0 {
		t.Errorf("enter = %+v, want key=%d", evs[1], browser.KeyEnter)
	}
	if evs[2].Key != browser.KeyF5 {
		t.Errorf("F5 = 0x%x, want %d", evs[2].Key, browser.KeyF5)
	}
	if evs[3].Key != browser.KeyPageDown {
		t.Errorf("pagedown = 0x%x, want %d", evs[3].Key, browser.KeyPageDown)
	}
	if evs[4].Key != browser.KeyUp {
		t.Errorf("↑ = 0x%x, want %d", evs[4].Key, browser.KeyUp)
	}
}

func TestInputKeyMappingRejectsGarbage(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)
	_, out, _ := post(t, s, "/debug/input", `[{"op":"key","key":"ctrl+"}]`)
	m := decode(t, out)
	if m["ok"] == true {
		t.Errorf("坏键名居然成功: %s", out)
	}
	results := m["results"].([]any)
	if len(results) != 1 || !strings.Contains(results[0].(map[string]any)["error"].(string), "key") {
		t.Errorf("results = %s, want 指出 key 的错误", out)
	}
	if len(f.events()) != 0 {
		t.Error("解析失败却注入了事件")
	}
}

func TestInputTextAndDropAndResize(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	_, out, _ := post(t, s, "/debug/input", `[
		{"op":"text","s":"你好，世界"},
		{"op":"drop","files":["C:\\a b\\图.png"]},
		{"op":"resize","w":640,"h":480}
	]`)
	if decode(t, out)["ok"] != true {
		t.Fatalf("响应 = %s", out)
	}
	evs := f.events()
	if len(evs) != 2 {
		t.Fatalf("注入 %d 个事件, want 2（文本 + 拖放）: %+v", len(evs), evs)
	}
	if evs[0].Kind != browser.InputTextInput || evs[0].Text != "你好，世界" {
		t.Errorf("文本事件 = %+v", evs[0])
	}
	if evs[1].Kind != browser.InputFilesDropped || len(evs[1].Files) != 1 ||
		evs[1].Files[0] != `C:\a b\图.png` {
		t.Errorf("拖放事件 = %+v", evs[1])
	}
	if f.resizeW != 640 || f.resizeH != 480 {
		t.Errorf("resize = %dx%d, want 640x480", f.resizeW, f.resizeH)
	}
}

func TestInputDragWalksThroughFrames(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	_, out, _ := post(t, s, "/debug/input", `[{"op":"drag","x":10,"y":10,"x2":110,"y2":10,"steps":3}]`)
	if decode(t, out)["ok"] != true {
		t.Fatalf("响应 = %s", out)
	}
	evs := f.events()
	// motion(start) + down + 3 motion + up
	if len(evs) != 6 {
		t.Fatalf("注入 %d 个事件, want 6: %+v", len(evs), evs)
	}
	if evs[0].Kind != browser.InputMouseMove || evs[1].Kind != browser.InputMouseDown {
		t.Errorf("拖拽应以移动+按下开头: %+v", evs[:2])
	}
	if evs[2].X != 43 || evs[3].X != 76 || evs[4].X != 110 {
		t.Errorf("中间点 = %d,%d,%d, want 43,76,110（分步位移才有位移量）", evs[2].X, evs[3].X, evs[4].X)
	}
	if evs[5].Kind != browser.InputMouseUp || evs[5].X != 110 {
		t.Errorf("抬起事件 = %+v", evs[5])
	}
}

func TestInputWaitIsRealTime(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)
	start := time.Now()
	post(t, s, "/debug/input", `[{"op":"wait","ms":120}]`)
	if d := time.Since(start); d < 100*time.Millisecond {
		t.Errorf("wait 120ms 只花了 %v：滚动动画按墙钟推进，wait 不能退化成空转", d)
	}
}

// TestInputSyncBarrier：sync=true（默认）时每个动作后插一道主循环栅栏，
// 关掉就不插——这是「脚本读到的状态是否已落地」的开关，必须钉住。
func TestInputSyncBarrier(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	body := `[{"op":"click","x":10,"y":10},{"op":"click","x":20,"y":20}]`
	post(t, s, "/debug/input", body)
	if got := f.muDoCalls(); got != 2 {
		t.Errorf("sync 默认开启时 Do 执行 %d 次, want 2（每个动作一道栅栏）", got)
	}

	f.reset()
	wrapped := `{"actions":[{"op":"click","x":10,"y":10},{"op":"click","x":20,"y":20}],"sync":false}`
	post(t, s, "/debug/input", wrapped)
	if got := f.muDoCalls(); got != 0 {
		t.Errorf("sync=false 时 Do 执行 %d 次, want 0", got)
	}
}

func (f *fakeWin) muDoCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.doCalls
}

func (f *fakeWin) reset() {
	f.mu.Lock()
	f.injected = nil
	f.doCalls = 0
	f.running = true
	f.mu.Unlock()
}

func TestInputObservationOps(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	// 命中坐标从布局取：#go 绝对定位在 #box 里，绝对坐标不是 CSS 写的
	// left/top（那是相对 #box 的），硬编码会点空。
	cx, cy := centerOf(t, s, "#go")
	_, out, _ := post(t, s, "/debug/input",
		fmt.Sprintf(`[{"op":"hit","x":%d,"y":%d},{"op":"query","selector":"#status"},{"op":"state"}]`, cx, cy))
	m := decode(t, out)
	if m["ok"] != true {
		t.Fatalf("响应 = %s", out)
	}
	results := m["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("results = %d 条, want 3", len(results))
	}
	hit := results[0].(map[string]any)["value"].(map[string]any)
	if chain := hit["chain"].([]any); len(chain) == 0 || chain[0].(map[string]any)["id"] != "go" {
		t.Errorf("hit 动作结果 = %v, want 链首 #go", hit["chain"])
	}
	q := results[1].(map[string]any)["value"].(map[string]any)
	if q["text"] != "就绪" {
		t.Errorf("query 动作结果 = %v", q)
	}
	st := results[2].(map[string]any)["value"].(map[string]any)
	if st["running"] != true {
		t.Errorf("state 动作结果 = %v", st)
	}
}

func TestInputStopsOnFirstError(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	_, out, _ := post(t, s, "/debug/input",
		`[{"op":"click","x":1,"y":1},{"op":"bogus-op"},{"op":"click","x":2,"y":2}]`)
	m := decode(t, out)
	if m["ok"] == true {
		t.Errorf("未知动作却报成功: %s", out)
	}
	results := m["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("results = %d 条, want 2（第三条不该执行）", len(results))
	}
	if int(results[1].(map[string]any)["index"].(float64)) != 1 {
		t.Errorf("失败位置 = %v, want index=1", results[1])
	}
	if len(f.events()) != 3 {
		t.Errorf("只应执行到第 1 个动作（3 个事件）: %+v", f.events())
	}

	// continue_on_error 让脚本能一口气收集所有失败项。
	f.reset()
	_, out, _ = post(t, s, "/debug/input",
		`{"actions":[{"op":"bad1"},{"op":"bad2"}],"continue_on_error":true}`)
	results = decode(t, out)["results"].([]any)
	if len(results) != 2 {
		t.Errorf("continue_on_error 下 results = %d 条, want 2: %s", len(results), out)
	}
}

func TestInputRequiresRunningLoop(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)
	f.mu.Lock()
	f.running = false
	f.mu.Unlock()

	code, out, _ := post(t, s, "/debug/input", `[{"op":"click","x":1,"y":1}]`)
	if code != http.StatusServiceUnavailable {
		t.Errorf("状态码 = %d: %s", code, out)
	}
	if !strings.Contains(string(out), "主循环") {
		t.Errorf("错误信息里没有指出主循环没跑: %s", out)
	}
	if len(f.events()) != 0 {
		t.Error("主循环没跑却注入了事件")
	}
}

func TestInputSingleActionAndBadBodies(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	// 单个动作对象（手写一条 curl 命令的最常见形态）。
	if _, out, _ := post(t, s, "/debug/input", `{"op":"nop"}`); decode(t, out)["ok"] != true {
		t.Errorf("单动作 = %s", out)
	}
	if code, out, _ := post(t, s, "/debug/input", ``); code != http.StatusBadRequest ||
		!strings.Contains(string(out), "没有动作") {
		t.Errorf("空 body = %d %s", code, out)
	}
	if code, _, _ := post(t, s, "/debug/input", `not json`); code != http.StatusBadRequest {
		t.Errorf("坏 JSON = %d, want 400", code)
	}
	// GET 打 POST 端点：要说清楚该用什么方法。
	if code, out, _ := get(t, s, "/debug/input"); code != http.StatusBadRequest ||
		!strings.Contains(string(out), "POST") {
		t.Errorf("GET /debug/input = %d %s", code, out)
	}
}

func TestInputCloseAndShotAndDocOps(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	dir := t.TempDir()
	path := strings.ReplaceAll(dir+`/shot.png`, `\`, `\\`)
	_, out, _ := post(t, s, "/debug/input", fmt.Sprintf(`[
		{"op":"doc","html":"<!doctype html><html><body><p id=\"p\">换页了</p></body></html>"},
		{"op":"query","selector":"#p"},
		{"op":"shot","path":"%s"},
		{"op":"close"}
	]`, path))
	m := decode(t, out)
	if m["ok"] != true {
		t.Fatalf("响应 = %s", out)
	}
	if f.closeReqs != 1 {
		t.Errorf("close 动作发出 %d 次关闭请求, want 1", f.closeReqs)
	}
	results := m["results"].([]any)
	q := results[1].(map[string]any)["value"].(map[string]any)
	if q["text"] != "换页了" {
		t.Errorf("doc 之后 query = %v", q)
	}
	data, err := os.ReadFile(dir + "/shot.png")
	if err != nil {
		t.Fatalf("截图没落盘: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("\x89PNG")) {
		t.Error("落盘内容不是 PNG")
	}
}

// ===== 帮助与首页 =====

func TestHelpAndIndex(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)

	_, out, _ := get(t, s, "/debug/help")
	m := decode(t, out)
	if len(m["endpoints"].([]any)) < 8 {
		t.Errorf("端点表太少: %v", m["endpoints"])
	}
	if len(m["actions"].([]any)) < 10 {
		t.Errorf("动作表太少: %v", m["actions"])
	}
	// 首页是人读的：每个端点都得列出来，否则curl 到根路径就看不到全貌。
	code, body, hdr := get(t, s, "/")
	if code != http.StatusOK || !strings.Contains(string(body), "/debug/state") {
		t.Errorf("首页 = %d %s", code, body)
	}
	if !strings.HasPrefix(hdr.Get("Content-Type"), "text/plain") {
		t.Errorf("首页 Content-Type = %q", hdr.Get("Content-Type"))
	}
}

func TestCloseIsIdempotentReported(t *testing.T) {
	f := newFakeWin(t)
	s := newTestServer(t, f)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); !errors.Is(err, ErrAlreadyClosed) {
		t.Errorf("重复 Close = %v, want ErrAlreadyClosed", err)
	}
}
