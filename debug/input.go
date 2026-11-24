// 本文件是输入动作的解释器：把 JSON 动作翻译成平台事件（经 InjectEvent，
// 与真人操作同一条解码链路），或主循环里的读/写动作。
//
// # 动作与「同步栅栏」
//
// 注入是**异步入队**的：SDL_PushEvent 把事件放进 SDL 队列，主循环下一帧
// 才取出来派发。于是「注入一个点击，紧接着读状态」会读到点击前的世界。
// 解决办法不是靠 sleep 猜，而是靠栅栏：往主循环投一个空任务并等它跑完——
// 主循环每帧先排空事件队列、后排空任务队列，所以空任务跑完就说明之前的
// 事件都已经派发过了。sync=true（默认）时每个动作后自动插一道栅栏。
//
// # 坐标与档位口径
//
//   - 坐标是视口像素（1 CSS px = 1 物理像素）；
//   - 滚轮是**档位**（整数），SDL 口径：dy 正 = 滚轮向上（滚动偏移减小，
//     露出更靠上的内容）；browser 层再按每档 100px 换算给引擎；
//   - 按键是 browser 包导出的键码（字母/数字即 ASCII，控制键用
//     browser.KeyEnter 等），修饰键用 browser.ModShift/ModCtrl/ModAlt。

package debug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-comm/ghromex/browser"
)

// dragStepDelay 是拖拽相邻两帧之间的间隔：拖选阈值是 3px，动画/选区推进
// 需要跨帧，太密（0ms）会把一次拖拽压进同一帧，看起来像没拖。
const dragStepDelay = 16 * time.Millisecond

// handleInput 执行一串动作。
//
// 响应体固定是 inputResp（HTTP 200）：动作逐条执行，失败的动作会在 results
// 里带 error 与序号——脚本需要知道「跑到第几步挂了」，而不是只拿到一句
// 「请求失败」。ok=false 表示有动作失败。
func (s *Server) handleInput(w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	actions, cont, sync, err := parseActions(body)
	if err != nil {
		return err
	}
	if len(actions) == 0 {
		return fmt.Errorf("debug: 没有动作（请求体应是动作数组，或 {\"actions\":[...]}）")
	}
	if !s.win.Running() {
		// 主循环没跑，注入的事件永远不会被消费。不假装成功——这正是
		// 「脚本跑了但窗口没反应」最常见的原因。
		return fmt.Errorf("%w：注入的事件不会被消费", browser.ErrLoopNotRunning)
	}

	resp := &inputResp{OK: true, Total: len(actions)}
	for i, a := range actions {
		res := actionResult{Index: i, Op: a.Op}
		val, err := s.runAction(a)
		if err != nil {
			res.Err = err.Error()
			resp.Results = append(resp.Results, res)
			resp.OK = false
			if !cont {
				break
			}
			continue
		}
		resp.Executed++
		if val != nil {
			res.Value = val
		}
		resp.Results = append(resp.Results, res)
		// close 之后不再插栅栏：窗口正在退出，主循环随时会停，
		// 等来的只会是「主循环已退出」——那是预期结果，不是失败。
		if sync && !strings.EqualFold(strings.TrimSpace(a.Op), "close") {
			if err := s.barrier(); err != nil {
				res.Err = "同步栅栏失败: " + err.Error()
				resp.Results[len(resp.Results)-1].Err = res.Err
				resp.OK = false
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

// parseActions 解析请求体。两种形态：
//
//	[{"op":"click","x":1,"y":2}, ...]              裸数组
//	{"actions":[...], "sync":true, "continue_on_error":false}
//
// 单个动作对象（{"op":"click",...}）也接受——脚本里手写一条最常见。
func parseActions(body []byte) (actions []action, continueOnError, sync bool, err error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, false, true, nil
	}
	sync = true
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(trimmed, &actions); err != nil {
			return nil, false, true, fmt.Errorf("debug: 动作数组解析失败: %w", err)
		}
		return actions, false, sync, nil
	case '{':
		var req inputReq
		if err := json.Unmarshal(trimmed, &req); err != nil {
			return nil, false, true, fmt.Errorf("debug: 请求体解析失败: %w", err)
		}
		if len(req.Actions) > 0 {
			sync = true
			if req.Sync != nil {
				sync = *req.Sync
			}
			return req.Actions, req.ContinueOnError, sync, nil
		}
		var one action
		if err := json.Unmarshal(trimmed, &one); err != nil {
			return nil, false, true, fmt.Errorf("debug: 请求体解析失败: %w", err)
		}
		if one.Op == "" {
			return nil, false, true, fmt.Errorf("debug: 请求体里没有动作（{\"actions\":[...]}）")
		}
		return []action{one}, false, sync, nil
	}
	return nil, false, true, fmt.Errorf("debug: 请求体应是动作数组或 {\"actions\":[...]}")
}

// runAction 执行一个动作，返回值只给观测类动作（state/hit/query/dom）用。
func (s *Server) runAction(a action) (any, error) {
	switch strings.ToLower(strings.TrimSpace(a.Op)) {
	case "move":
		x, y, err := s.point(a)
		if err != nil {
			return nil, err
		}
		return nil, s.inject(browser.InputEvent{Kind: browser.InputMouseMove, X: x, Y: y})

	case "down", "up":
		x, y, err := s.point(a)
		if err != nil {
			return nil, err
		}
		btn, err := buttonOf(a.Button)
		if err != nil {
			return nil, err
		}
		kind := browser.InputMouseDown
		if strings.EqualFold(a.Op, "up") {
			kind = browser.InputMouseUp
		}
		return nil, s.inject(browser.InputEvent{Kind: kind, Button: btn, X: x, Y: y})

	case "click":
		x, y, err := s.point(a)
		if err != nil {
			return nil, err
		}
		btn, err := buttonOf(a.Button)
		if err != nil {
			return nil, err
		}
		// 先补一条 motion：真实点击前光标就在那儿，悬停态与光标形状
		// （CSS cursor）要在按下之前就位。
		if err := s.inject(browser.InputEvent{Kind: browser.InputMouseMove, X: x, Y: y}); err != nil {
			return nil, err
		}
		if err := s.inject(browser.InputEvent{Kind: browser.InputMouseDown, Button: btn, X: x, Y: y}); err != nil {
			return nil, err
		}
		return nil, s.inject(browser.InputEvent{Kind: browser.InputMouseUp, Button: btn, X: x, Y: y})

	case "drag":
		x, y, err := s.point(a)
		if err != nil {
			return nil, err
		}
		x2, y2 := x, y
		if a.X2 != nil {
			x2 = *a.X2
		}
		if a.Y2 != nil {
			y2 = *a.Y2
		}
		steps := 8
		if a.Steps != nil && *a.Steps > 0 {
			steps = *a.Steps
		}
		btn, err := buttonOf(a.Button)
		if err != nil {
			return nil, err
		}
		if err := s.inject(browser.InputEvent{Kind: browser.InputMouseMove, X: x, Y: y}); err != nil {
			return nil, err
		}
		if err := s.inject(browser.InputEvent{Kind: browser.InputMouseDown, Button: btn, X: x, Y: y}); err != nil {
			return nil, err
		}
		for i := 1; i <= steps; i++ {
			// 每步之间真睡一会儿：拖选阈值是 3px，引擎的拖选/滚动动画
			// 都要跨帧推进，全塞进同一帧就等于一次瞬移。
			time.Sleep(dragStepDelay)
			mx := x + (x2-x)*i/steps
			my := y + (y2-y)*i/steps
			if err := s.inject(browser.InputEvent{Kind: browser.InputMouseMove, X: mx, Y: my}); err != nil {
				return nil, err
			}
		}
		return nil, s.inject(browser.InputEvent{Kind: browser.InputMouseUp, Button: btn, X: x2, Y: y2})

	case "wheel":
		x, y, err := s.point(a)
		if err != nil {
			return nil, err
		}
		dx, dy := 0, 0
		if a.DX != nil {
			dx = *a.DX
		}
		if a.DY != nil {
			dy = *a.DY
		}
		return nil, s.inject(browser.InputEvent{Kind: browser.InputWheel, WheelX: dx, WheelY: dy, X: x, Y: y})

	case "key":
		key, mods, err := keyOf(a)
		if err != nil {
			return nil, err
		}
		return nil, s.inject(browser.InputEvent{Kind: browser.InputKeyDown, Key: key, Mods: mods})

	case "text":
		txt := a.Text
		if txt == "" {
			txt = a.S
		}
		if txt == "" {
			return nil, fmt.Errorf("debug: text 动作需要 text 或 s 字段")
		}
		return nil, s.inject(browser.InputEvent{Kind: browser.InputTextInput, Text: txt})

	case "drop":
		if len(a.Files) == 0 {
			return nil, fmt.Errorf("debug: drop 动作需要 files 数组")
		}
		return nil, s.inject(browser.InputEvent{Kind: browser.InputFilesDropped, Files: a.Files})

	case "wait", "sleep":
		ms := 100
		if a.MS != nil {
			ms = *a.MS
		}
		if ms < 0 {
			ms = 0
		}
		// 真实睡眠（不是帧数）：滚动动画与气泡提示都按墙钟推进，
		// 等「多少毫秒」才与用户看到的一致。
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return nil, nil

	case "sync":
		return nil, nil // 栅栏由外层统一插，这里只是让脚本能显式写出来

	case "state":
		v, err := s.main(func() (any, error) { return s.readState(), nil })
		return v, err

	case "hit":
		x, y, err := s.point(a)
		if err != nil {
			return nil, err
		}
		v, err := s.main(func() (any, error) { return s.readHit(x, y) })
		return v, err

	case "query", "dom":
		sel := a.Sel
		if sel == "" {
			return nil, fmt.Errorf("debug: %s 动作需要 selector 字段", a.Op)
		}
		if strings.EqualFold(a.Op, "dom") {
			v, err := s.main(func() (any, error) { return s.readDOM(sel, defaultDepth, defaultNodeLimit) })
			return v, err
		}
		v, err := s.main(func() (any, error) { return s.readText(sel) })
		return v, err

	case "doc":
		src, err := docSource(a.HTML, a.Path)
		if err != nil {
			return nil, err
		}
		v, err := s.main(func() (any, error) {
			if _, err := s.win.OpenDocument(src); err != nil {
				return nil, fmt.Errorf("debug: 打开文档失败: %w", err)
			}
			return s.readState(), nil
		})
		return v, err

	case "resize":
		w, h := 0, 0
		if a.W != nil {
			w = *a.W
		}
		if a.H != nil {
			h = *a.H
		}
		if w <= 0 || h <= 0 {
			return nil, fmt.Errorf("debug: resize 动作需要正的 w 与 h")
		}
		_, err := s.main(func() (any, error) {
			s.win.Resize(w, h)
			return nil, nil
		})
		return nil, err

	case "shot":
		v, err := s.main(func() (any, error) { return s.win.ScreenshotPNG() })
		if err != nil {
			return nil, err
		}
		b := v.([]byte)
		if a.Path == "" {
			return map[string]any{"bytes": len(b)}, nil
		}
		p := a.Path
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			return nil, fmt.Errorf("debug: 写截图失败: %w", err)
		}
		return map[string]any{"path": p, "bytes": len(b)}, nil

	case "close":
		s.win.RequestClose()
		return nil, nil

	case "nop", "":
		return nil, nil
	}
	return nil, fmt.Errorf("debug: 未知动作 %q（动作清单见 /debug/help）", a.Op)
}

// docSource 把「html 或 path」解析成 HTML 源码。
//
// 读文件在调用方 goroutine 上做（不占主循环）；相对路径按宿主的工作目录
// 解析（命令行里传 ./page.html 才符合直觉）。
func docSource(html, path string) (string, error) {
	if strings.TrimSpace(html) != "" {
		return html, nil
	}
	if path == "" {
		return "", fmt.Errorf("debug: 需要 html 或 path 字段")
	}
	p := path
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("debug: 读取文档失败: %w", err)
	}
	return string(data), nil
}

// point 取出动作的坐标：没给就用最后已知位置（再没有就用视口中心）。
func (s *Server) point(a action) (int, int, error) {
	if a.X == nil && a.Y == nil {
		x, y := s.syncPoint()
		return x, y, nil
	}
	x, y := 0, 0
	if a.X != nil {
		x = *a.X
	}
	if a.Y != nil {
		y = *a.Y
	}
	s.notePoint(x, y)
	return x, y, nil
}

// inject 注入一个输入事件并把「主循环没跑」的错误收敛成带上下文的报错。
//
// 改成返回 error 是有意的：`target.InjectInput` 报错说明事件根本没进队列，
// 动作必须失败——否则脚本会拿到「成功」但窗口毫无反应。
func (s *Server) inject(ev browser.InputEvent) error {
	if err := s.win.InjectInput(ev); err != nil {
		return fmt.Errorf("注入事件失败: %w", err)
	}
	return nil
}

// buttonOf 解析按键名。
func buttonOf(name *string) (int, error) {
	if name == nil || *name == "" {
		return browser.MouseButtonLeft, nil
	}
	switch strings.ToLower(strings.TrimSpace(*name)) {
	case "left", "1":
		return browser.MouseButtonLeft, nil
	case "middle", "2":
		return browser.MouseButtonMiddle, nil
	case "right", "3":
		return browser.MouseButtonRight, nil
	}
	return 0, fmt.Errorf("debug: 未知鼠标键 %q（left/middle/right）", *name)
}

// keyOf 解析按键：既接受 "ctrl+shift+s" 这样的名字串，也接受原始键码 + 掩码。
func keyOf(a action) (int, uint16, error) {
	if a.Code != nil {
		key := *a.Code
		var mods uint16
		if a.Mods != nil {
			mods = uint16(*a.Mods)
		}
		return key, mods, nil
	}
	if strings.TrimSpace(a.Key) == "" {
		return 0, 0, fmt.Errorf("debug: key 动作需要 key（或 code+mods）")
	}
	var mods uint16
	parts := strings.Split(a.Key, "+")
	keyName := ""
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if m, ok := modOf(p); ok {
			mods |= m
			continue
		}
		keyName = p
	}
	if keyName == "" {
		return 0, 0, fmt.Errorf("debug: key %q 里没有键名", a.Key)
	}
	key, ok := keyCodeOf(keyName)
	if !ok {
		return 0, 0, fmt.Errorf("debug: 未知按键名 %q（可用：字母/数字、enter、tab、esc、space、backspace、delete、home、end、pageup、pagedown、方向键、insert、F1~F12）", keyName)
	}
	return key, mods, nil
}

// modOf 解析修饰键名。
func modOf(name string) (uint16, bool) {
	switch strings.ToLower(name) {
	case "ctrl", "control":
		return browser.ModCtrl, true
	case "shift":
		return browser.ModShift, true
	case "alt":
		return browser.ModAlt, true
	}
	return 0, false
}

// keyCodeOf 把键名翻成 browser 包导出的键码。
//
// 字母与数字用 ASCII（SDL 的 SDLK_a 就是 'a'）；控制键用 browser.Key*
// 常量（与引擎 text_input.go 的 Key* 同值）。browser 包自带的这些常量
// 让「调 debug / 调 InjectInput」都不必 import engine。
func keyCodeOf(name string) (int, bool) {
	switch strings.ToLower(name) {
	case "enter", "return":
		return browser.KeyEnter, true
	case "tab":
		return browser.KeyTab, true
	case "esc", "escape":
		return browser.KeyEscape, true
	case "backspace", "bs":
		return browser.KeyBackspace, true
	case "delete", "del":
		return browser.KeyDelete, true
	case "space":
		return ' ', true
	case "home":
		return browser.KeyHome, true
	case "end":
		return browser.KeyEnd, true
	case "left", "←":
		return browser.KeyLeft, true
	case "right", "→":
		return browser.KeyRight, true
	case "up", "↑":
		return browser.KeyUp, true
	case "down", "↓":
		return browser.KeyDown, true
	case "pageup", "pgup":
		return browser.KeyPageUp, true
	case "pagedown", "pgdn", "pgdown":
		return browser.KeyPageDown, true
	case "insert", "ins":
		return browser.KeyInsert, true
	}
	// F1~F12。
	if len(name) >= 2 && (name[0] == 'f' || name[0] == 'F') {
		if n, err := strconv.Atoi(name[1:]); err == nil && n >= 1 && n <= 12 {
			return browser.KeyF1 + (n - 1), true
		}
	}
	// 单字符：字母/数字/常用符号按 ASCII 走。
	r := []rune(name)
	if len(r) == 1 {
		return int(r[0]), true
	}
	return 0, false
}
