// 本文件是调试服务的 HTTP 层：地址收敛、路由、JSON 出口，以及「一切触碰
// 文档与渲染的读写都交回主循环执行」这条纪律的落地点。

package debug

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-comm/ghromex/browser"
	"github.com/go-comm/ghromex/engine"
)

// 默认口径。
const (
	// defaultAddr 只绑回环、端口交给系统分配：调试服务不该和宿主已有的
	// 端口撞车，也不该被局域网访问到。
	defaultAddr = "127.0.0.1:0"
	// defaultTimeout 是单次「交回主循环执行」的等待上限。参照物：42 万元素
	// 页面的全量重排是 1.7 秒，3 秒留了余量，又不至于让请求吊死。
	defaultTimeout = 3 * time.Second
	// maxBody 是请求体上限。一页 HTML 塞进 JSON 也远小于它；超限直接拒，
	// 免得有人把调试端口当上传口用。
	maxBody = 8 << 20
)

// EnvPort 是起服务的环境变量：值为端口号或 host:port，未设置表示不起。
// 详见 ServeFromEnv。
const EnvPort = "GHROMEX_DEBUG_PORT"

// 本包哨兵错误。
var (
	// ErrNotLoopback 表示监听地址不是回环地址。调试端口能改文档、能注入输入、
	// 能读画面，绑到非回环网卡上等于把本机窗口交给局域网里任何人。
	ErrNotLoopback = errors.New("debug: 调试服务只允许绑定回环地址（127.0.0.1 / localhost / ::1）")
	// ErrAlreadyClosed 表示服务已关闭后再调 Close。
	ErrAlreadyClosed = errors.New("debug: 服务已关闭")
	// errNoDocument 表示请求需要一个已打开的文档，而窗口还没有。
	errNoDocument = errors.New("debug: 窗口还没有文档（先 POST /debug/doc）")
)

// Options 是起服务的参数。零值即可用（随机端口、只绑回环）。
type Options struct {
	// Addr 是监听地址，默认 "127.0.0.1:0"（随机端口）。裸端口号（"9223"）
	// 按 127.0.0.1 解释。**非回环地址会被拒绝**（ErrNotLoopback）。
	Addr string
	// Timeout 是单次「交回主循环执行」的等待上限，默认 3s。
	// 超时说明主循环正卡在长布局/长绘制里，或 Run 已经退出。
	Timeout time.Duration
	// Logf 收诊断输出（启动地址）。默认写 os.Stderr 并带包名前缀；
	// 传一个空函数就完全静默。
	Logf func(format string, args ...any)
}

// target 是调试服务用到的窗口能力集。
//
// *browser.Window 直接满足，不需要适配层；测试用假实现覆盖 HTTP 层与动作
// 解释，于是这个包的端到端用例在没有 SDL 的环境下也能跑。
//
// 注释里标了「主线程」的方法只能在主循环 goroutine 上调用——本包的 handler
// 一律把它们包进 Do 的任务里。
type target interface {
	// —— 跨线程安全 ——
	Do(f func() (any, error), timeout time.Duration) (any, error)
	Running() bool
	InjectInput(ev browser.InputEvent) error // 事件在主循环下一帧被消费
	RequestClose()

	// —— 必须主线程（包在 Do 任务里）——
	Document() engine.HTMLDocument
	OpenDocument(src string) (engine.HTMLDocument, error)
	ViewportWidth() int
	ViewportHeight() int
	Resize(w, h int)
	ScreenshotPNG() ([]byte, error)
}

// Server 是一个已启动的调试服务。
type Server struct {
	win  target
	opt  Options
	ln   net.Listener
	srv  *http.Server
	logf func(string, ...any)

	mu     sync.Mutex // 保护 closed
	closed bool
	posMu  sync.Mutex // 保护最后已知鼠标位置
	lastX  int
	lastY  int
	mouseX bool
}

// Serve 在 win 上起调试服务并立刻开始监听（请求在后台 goroutine 处理）。
//
// 调用时机：Run 之前调也行（服务本身不碰窗口），但请求真正有意义要等主循环
// 跑起来——Run 没跑时，状态类请求返回「主循环未在运行」，注入的事件也不会被
// 消费。服务不假装成功，这对排查「脚本跑了但窗口没反应」是必要的信息。
//
// 失败（端口被占、地址不是回环）返回错误且不起服务。返回的 Server 用 Close
// 停掉；不关也不影响进程退出（Run 返回后服务只是不再有意义）。
func Serve(win *browser.Window, opt Options) (*Server, error) {
	return serve(win, opt)
}

// ServeFromEnv 按环境变量 GHROMEX_DEBUG_PORT 起服务：
//
//	GHROMEX_DEBUG_PORT=9223            固定端口 127.0.0.1:9223
//	GHROMEX_DEBUG_PORT=0               随机端口（启动时打印实际地址）
//	GHROMEX_DEBUG_PORT=127.0.0.1:9223  显式 host:port
//	未设置 / 空值                       不起服务，返回 (nil, nil)
//
// 值非法（端口越界、非回环地址）返回错误——调试开关也要拒绝「以为开了其实
// 没开」，调用方直接把错误打出来即可。
func ServeFromEnv(win *browser.Window) (*Server, error) {
	v := strings.TrimSpace(os.Getenv(EnvPort))
	if v == "" {
		return nil, nil
	}
	return Serve(win, Options{Addr: v})
}

// serve 是真正的入口（收 target，便于测试注入假窗口）。
func serve(win target, opt Options) (*Server, error) {
	addr, err := normalizeAddr(opt.Addr)
	if err != nil {
		return nil, err
	}
	if opt.Timeout <= 0 {
		opt.Timeout = defaultTimeout
	}
	opt.Addr = addr
	logf := opt.Logf
	if logf == nil {
		logf = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "ghromex/debug: "+format+"\n", args...)
		}
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("debug: 监听 %s: %w", addr, err)
	}
	s := &Server{win: win, opt: opt, ln: ln, logf: logf}
	mux := http.NewServeMux()
	get := map[string]handlerFunc{
		"/debug/state":      s.handleState,
		"/debug/hit":        s.handleHit,
		"/debug/dom":        s.handleDOM,
		"/debug/text":       s.handleText,
		"/debug/attr":       s.handleAttr,
		"/debug/shot.png":   s.handleShot,
		"/debug/layout.svg": s.handleLayout,
		"/debug/help":       s.handleHelp,
	}
	for p, h := range get {
		mux.HandleFunc(p, s.wrap(h))
	}
	for p, h := range map[string]handlerFunc{
		"/debug/doc":   s.handleDoc,
		"/debug/input": s.handleInput,
	} {
		mux.HandleFunc(p, s.requireMethod(http.MethodPost, h))
	}
	mux.HandleFunc("/debug", s.wrap(s.handleIndex))
	mux.HandleFunc("/debug/", s.wrap(s.handleIndex))
	mux.HandleFunc("/", s.wrap(s.handleIndex))
	s.srv = &http.Server{Handler: mux}
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logf("服务异常退出: %v", err)
		}
	}()
	s.logf("调试服务已启动 %s （端点清单见 %s）", s.URL("/debug/"), s.URL("/debug/help"))
	return s, nil
}

// Addr 返回实际监听地址（随机端口场景下才有意义）。
func (s *Server) Addr() string { return s.ln.Addr().String() }

// URL 拼出可以直接贴进地址栏/curl 的完整地址，path 形如 "/debug/state"。
func (s *Server) URL(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "http://" + s.Addr() + path
}

// Close 停止服务并断开监听。重复调用返回 ErrAlreadyClosed。
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrAlreadyClosed
	}
	s.closed = true
	return s.srv.Close()
}

// normalizeAddr 把用户给的地址收敛成「回环 + 端口」，并拒绝非回环。
func normalizeAddr(a string) (string, error) {
	a = strings.TrimSpace(a)
	if a == "" {
		a = defaultAddr
	}
	// 裸端口号（"9223"、"0"）按 127.0.0.1 解释：命令行与环境变量里最常见。
	// IPv6 字面量（"[::1]:9223"）带方括号，不含裸冒号歧义，SplitHostPort 能解。
	if !strings.Contains(strings.TrimPrefix(a, "["), ":") {
		p, err := strconv.Atoi(a)
		if err != nil || p < 0 || p > 65535 {
			return "", fmt.Errorf("debug: 端口不合法: %q", a)
		}
		a = "127.0.0.1:" + strconv.Itoa(p)
	}
	host, port, err := net.SplitHostPort(a)
	if err != nil {
		return "", fmt.Errorf("debug: 地址不合法 %q: %w", a, err)
	}
	switch strings.ToLower(host) {
	case "", "127.0.0.1", "localhost", "::1":
		if host == "" {
			host = "127.0.0.1"
		}
	default:
		return "", fmt.Errorf("%w（收到 %q）", ErrNotLoopback, a)
	}
	return net.JoinHostPort(host, port), nil
}

// main 把一个读/改文档的闭包交回主循环执行，超时用 Options.Timeout。
func (s *Server) main(f func() (any, error)) (any, error) {
	return s.win.Do(f, s.opt.Timeout)
}

// barrier 排空此前注入的事件：往主循环投一个空任务并等它执行。
//
// 主循环每帧先排空 SDL 事件队列、后排空任务队列，所以「空任务跑完」这个
// 事实就说明：**之前**注入的输入都已经派发过了。外部脚本因此不必靠
// sleep 猜「事件落地了没」。
func (s *Server) barrier() error {
	_, err := s.main(func() (any, error) { return nil, nil })
	return err
}

// syncPoint 返回缺省落点：最后已知的鼠标位置；一次都没记过就用视口中心
// （视口尺寸必须在主线程读，所以这条走一次 Do）。
func (s *Server) syncPoint() (int, int) {
	s.posMu.Lock()
	if s.mouseX {
		x, y := s.lastX, s.lastY
		s.posMu.Unlock()
		return x, y
	}
	s.posMu.Unlock()
	v, err := s.main(func() (any, error) {
		return [2]int{s.win.ViewportWidth() / 2, s.win.ViewportHeight() / 2}, nil
	})
	if err != nil {
		return 0, 0
	}
	p := v.([2]int)
	return p[0], p[1]
}

// notePoint 记住一次动作里的坐标，供省略坐标的后续动作复用。
func (s *Server) notePoint(x, y int) {
	s.posMu.Lock()
	s.lastX, s.lastY, s.mouseX = x, y, true
	s.posMu.Unlock()
}

// handlerFunc 是能返回错误的 handler：错误统一走 JSON 外壳，
// 免得每个端点自己拼「失败长什么样」。
type handlerFunc func(w http.ResponseWriter, r *http.Request) error

// wrap 把 handlerFunc 适配成 http.HandlerFunc。
func (s *Server) wrap(h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			s.writeErr(w, err)
		}
	}
}

// requireMethod 在方法不符时给出明确错误（GET 打 POST 端点是最常见的误用）。
func (s *Server) requireMethod(method string, h handlerFunc) http.HandlerFunc {
	return s.wrap(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != method {
			return fmt.Errorf("debug: %s 只接受 %s（收到 %s）", r.URL.Path, method, r.Method)
		}
		return h(w, r)
	})
}

// writeErr 把本包与 browser 的哨兵错误映射成状态码 + 下一步提示。
func (s *Server) writeErr(w http.ResponseWriter, err error) {
	hint := ""
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, browser.ErrLoopNotRunning):
		status, hint = http.StatusServiceUnavailable, "主循环没在跑：先 Window.Run()"
	case errors.Is(err, browser.ErrTaskTimeout):
		status, hint = http.StatusGatewayTimeout, "主循环正忙（长布局/长绘制），或窗口已退出 Run"
	case errors.Is(err, browser.ErrLoopStopped):
		status, hint = http.StatusServiceUnavailable, "窗口在主循环执行任务前就退出了"
	case errors.Is(err, errNoDocument):
		status, hint = http.StatusNotFound, "先 POST /debug/doc 打开一个文档"
	}
	writeJSON(w, status, errResp{OK: false, Error: err.Error(), Hint: hint})
}

// errResp 是错误响应的外壳。
type errResp struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	Hint  string `json:"hint,omitempty"`
}

// writeJSON 输出 JSON（紧凑、不做 HTML 转义：中文与 <tag> 原样出现，
// curl 与 jq 读起来都省事）。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "ghromex/debug: 响应编码失败: %v\n", err)
	}
}

// readBody 读请求体并限流。
func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("读取请求体: %w", err)
	}
	return b, nil
}

// ===== 观测端点 =====

// handleState 汇报窗口与文档的当前状态。
func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) error {
	v, err := s.main(func() (any, error) { return s.readState(), nil })
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) readState() any {
	st := stateResp{OK: true, Running: s.win.Running()}
	st.Viewport = box{W: s.win.ViewportWidth(), H: s.win.ViewportHeight()}
	doc := s.win.Document()
	if doc == nil {
		return st
	}
	st.Document = &docInfo{
		Title:           doc.Title(),
		ChangeCount:     doc.ChangeCount(),
		PaintCount:      engine.PaintCount(doc),
		ScrollAnimating: engine.ScrollAnimating(doc),
		TooltipPending:  engine.TooltipPending(doc),
	}
	x, y := engine.ScrollOffset(doc)
	st.Document.Scroll = &scrollBox{X: x, Y: y}
	st.Hover = summarize(engine.HoveredElement(doc))
	st.Focus = summarize(engine.FocusedElement(doc))
	st.Active = summarize(engine.ActiveElement(doc))
	if text, ok := engine.SelectedText(doc); ok {
		n, _ := engine.SelectionLength(doc)
		st.Selection = &selectionInfo{Text: clipRunes(text, 200), Length: n}
	}
	return st
}

// handleHit 做命中测试并给出父链——回答「这个点到底落在谁身上、为什么
// 点了没反应」。
func (s *Server) handleHit(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	x, err := parseInt(q.Get("x"), "x")
	if err != nil {
		return err
	}
	y, err := parseInt(q.Get("y"), "y")
	if err != nil {
		return err
	}
	v, err := s.main(func() (any, error) { return s.readHit(x, y) })
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) readHit(x, y int) (any, error) {
	doc := s.win.Document()
	if doc == nil {
		return nil, errNoDocument
	}
	hover := engine.HoveredElement(doc)
	focus := engine.FocusedElement(doc)
	active := engine.ActiveElement(doc)
	hit := &hitResp{OK: true, X: x, Y: y}
	s.notePoint(x, y)
	for el := engine.ElementAt(doc, x, y); el != nil; el = el.ParentElement() {
		e := summarize(el)
		if e == nil {
			break
		}
		e.Hovered = same(el, hover)
		e.Focused = same(el, focus)
		e.Active = same(el, active)
		hit.Chain = append(hit.Chain, *e)
		if len(hit.Chain) >= maxChain {
			hit.Truncated = true
			break
		}
	}
	hit.Cursor = cursorName(engine.CursorAt(doc, x, y))
	return hit, nil
}

// handleText 返回选择器命中文本的元素文本与取值。
func (s *Server) handleText(w http.ResponseWriter, r *http.Request) error {
	sel := r.URL.Query().Get("selector")
	if sel == "" {
		return fmt.Errorf("debug: /debug/text 需要 selector 参数")
	}
	v, err := s.main(func() (any, error) { return s.readText(sel) })
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) readText(sel string) (any, error) {
	el, err := s.querySelector(sel)
	if err != nil {
		return nil, err
	}
	return &textResp{
		OK:       true,
		Selector: sel,
		Tag:      el.TagName(),
		Text:     el.InnerText(),
		Value:    engine.GetValue(el),
	}, nil
}

// handleAttr 读单个属性：DOM 树里的属性是白名单，任意属性走这里取。
func (s *Server) handleAttr(w http.ResponseWriter, r *http.Request) error {
	sel := r.URL.Query().Get("selector")
	name := r.URL.Query().Get("name")
	if sel == "" || name == "" {
		return fmt.Errorf("debug: /debug/attr 需要 selector 与 name 参数")
	}
	v, err := s.main(func() (any, error) {
		el, err := s.querySelector(sel)
		if err != nil {
			return nil, err
		}
		return &attrResp{OK: true, Selector: sel, Name: name, Value: el.GetAttribute(name)}, nil
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

// handleDOM 输出 DOM 树快照（可选子树、深度、节点上限）。
func (s *Server) handleDOM(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	sel := q.Get("selector")
	depth, err := parseIntDefault(q.Get("depth"), "depth", defaultDepth)
	if err != nil {
		return err
	}
	limit, err := parseIntDefault(q.Get("limit"), "limit", defaultNodeLimit)
	if err != nil {
		return err
	}
	v, err := s.main(func() (any, error) { return s.readDOM(sel, depth, limit) })
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) readDOM(sel string, depth, limit int) (any, error) {
	doc := s.win.Document()
	if doc == nil {
		return nil, errNoDocument
	}
	resp := &domResp{OK: true, Selector: sel}
	var root engine.HTMLElement = doc
	if sel != "" {
		el := doc.QuerySelector(sel)
		if el == nil {
			return nil, fmt.Errorf("debug: 选择器无匹配元素: %q", sel)
		}
		root = el
	}
	n := 0
	resp.Tree = buildDOM(root, depth, limit, &n, &resp.Truncated)
	resp.Count = n
	return resp, nil
}

// handleLayout 导出当前布局的 SVG（与 -dump-svg 同源）。
func (s *Server) handleLayout(w http.ResponseWriter, _ *http.Request) error {
	v, err := s.main(func() (any, error) {
		doc := s.win.Document()
		if doc == nil {
			return nil, errNoDocument
		}
		return doc.DumpSVG(), nil
	})
	if err != nil {
		return err
	}
	writeBytes(w, "image/svg+xml; charset=utf-8", []byte(v.(string)))
	return nil
}

// handleShot 回读当前画面并出 PNG。
func (s *Server) handleShot(w http.ResponseWriter, _ *http.Request) error {
	v, err := s.main(func() (any, error) { return s.win.ScreenshotPNG() })
	if err != nil {
		return err
	}
	writeBytes(w, "image/png", v.([]byte))
	return nil
}

func writeBytes(w http.ResponseWriter, contentType string, b []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	_, _ = w.Write(b)
}

// handleDoc 换文档：body 是 {"html":"..."} 或 {"path":"..."}。
//
// 读文件在 handler goroutine 上做（不占主循环），解析+首布局在任务里做
// （那是必须主线程的部分，也是真的耗时部分）。
func (s *Server) handleDoc(w http.ResponseWriter, r *http.Request) error {
	var in docReq
	b, err := readBody(r)
	if err != nil {
		return err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &in); err != nil {
			return fmt.Errorf("debug: 请求体不是合法 JSON: %w", err)
		}
	}
	src, err := docSource(in.HTML, in.Path)
	if err != nil {
		return err
	}
	v, err := s.main(func() (any, error) {
		if _, err := s.win.OpenDocument(src); err != nil {
			return nil, fmt.Errorf("debug: 打开文档失败: %w", err)
		}
		return s.readState(), nil
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

// handleHelp 给机器读的端点清单与动作表（curl 一把就能开始试）。
func (s *Server) handleHelp(w http.ResponseWriter, _ *http.Request) error {
	writeJSON(w, http.StatusOK, helpResp{
		OK:        true,
		Endpoints: helpEndpoints,
		Actions:   helpActions,
		Notes: []string{
			"坐标是视口像素，1 CSS px = 1 物理像素",
			"滚轮 dy/dx 是档位，SDL 口径：正 = 向上/向右",
			"每个输入动作之后服务会做一次主循环栅栏，下一个动作看到的就是落地后的状态",
			"注入走 SDL_PushEvent，与真人操作同一条解码链路",
			"服务只绑回环、默认不开、不开 CORS",
		},
	})
	return nil
}

// handleIndex 是人读的首页：一行一个端点，点开就能看。
func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, "ghromex 调试服务\n\n")
	for _, e := range helpEndpoints {
		fmt.Fprintf(w, "  %-22s %s\n", e.Method+" "+e.Path, e.Desc)
	}
	fmt.Fprintf(w, "\n输入动作见 %s\n", s.URL("/debug/help"))
	return nil
}
