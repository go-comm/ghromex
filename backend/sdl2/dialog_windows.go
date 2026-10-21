//go:build windows

// 本文件是 Windows 原生文件对话框（Shell 的 IFileDialog）的实现；
// 包文档见 doc.go。
//
// 为什么用 IFileDialog 而不是 comdlg32 的 GetOpenFileNameW：后者是
// Win9x 风格的老对话框（没有左边的导航树、没有「另存为」的现代布局），
// 而且没法选目录。IFileDialog 是 Vista 起的现代对话框，open/save/pick
// folder 三种形态共用一个实现，筛选器也是结构化的（COMDLG_FILTERSPEC）。
//
// IFileDialog 是 COM 接口，Go 没有现成绑定，这里用 syscall 按 vtable
// 直接调用——纯 Go、无 CGO，与本后端动态加载 SDL2.dll 的做法一致。
//
// 线程约束：IFileDialog 必须在 STA（单线程套间）上创建并 Show。入口处
// LockOSThread 并用 CoInitializeEx(COINIT_APARTMENTTHREADED) 把当前线程
// 初始化成 STA；Show 期间会阻塞该线程（对话框自带消息循环，SDL 事件
// 循环随之暂停，这与原生应用「模态对话框期间父窗口不响应」一致）。
//
// vtable 索引以 Windows SDK 的 ShObjIdl_core.idl 为准（接口方法按声明顺序
// 进虚表）：IUnknown 占 0..2（QueryInterface/AddRef/Release），
// IModalWindow::Show 是 3，IFileDialog 的方法从 4 起，IFileOpenDialog /
// IFileSaveDialog 在其后追加。索引写错会调到相邻方法（可能直接崩），
// 改之前先核对 IDL。

package sdl2

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// FileFilter 描述文件对话框里的一个类型过滤器（对应下拉框里的一项）。
type FileFilter struct {
	// Name 是显示名，如 "文本文件 (*.txt)"。
	Name string
	// Pattern 是扩展名模式，如 "*.txt"；多个模式用 ";" 分隔
	//（"*.txt;*.md"）。
	Pattern string
}

// 系统 DLL 与入口点。Shell 的文件对话框不经 SDL，这里直接动态加载
// 系统 DLL（与 SDL2.dll 一样不引入 CGO）。
var (
	dllOle32   = windows.NewLazySystemDLL("ole32.dll")
	dllShell32 = windows.NewLazySystemDLL("shell32.dll")

	procCoInitializeEx              = dllOle32.NewProc("CoInitializeEx")
	procCoUninitialize              = dllOle32.NewProc("CoUninitialize")
	procCoCreateInstance            = dllOle32.NewProc("CoCreateInstance")
	procCoTaskMemFree               = dllOle32.NewProc("CoTaskMemFree")
	procSHCreateItemFromParsingName = dllShell32.NewProc("SHCreateItemFromParsingName")
)

// guid 是 Windows 的 GUID（16 字节，字段顺序与 C 结构一致）。
type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// Shell 文件对话框相关的 CLSID / IID（shobjidl_core.h / ShObjIdl_core.idl）。
//
// 取值以 Windows SDK 的 IDL 为准：CLSID_FileOpenDialog 与 CLSID_FileSaveDialog
// 见 DEFINE_GUID，IID 见接口的 uuid 属性。
var (
	clsidFileOpenDialog = guid{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	clsidFileSaveDialog = guid{0xC0B4E2F3, 0xBA21, 0x4773, [8]byte{0x8D, 0xBA, 0x33, 0x5E, 0xC9, 0x46, 0xEB, 0x8B}}
	iidFileOpenDialog   = guid{0xD57C7288, 0xD4AD, 0x4768, [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	iidFileSaveDialog   = guid{0x84BCCD23, 0x5FDE, 0x4CDB, [8]byte{0xAE, 0xA4, 0xAF, 0x64, 0xB8, 0x3D, 0x78, 0xAB}}
	iidShellItem        = guid{0x43826D1E, 0xE718, 0x42EE, [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}
)

// COM / Shell 常量。
const (
	coinitApartmentThreaded = 0x2 // COINIT_APARTMENTTHREADED（STA）
	clsctxInprocServer      = 0x1 // CLSCTX_INPROC_SERVER

	// FILEOPENDIALOGOPTIONS
	fosOverwritePrompt = 0x00000002 // 覆盖已有文件时询问
	fosStrictFileTypes = 0x00000004 // 只允许过滤器里列出的类型
	fosNoChangeDir     = 0x00000008 // 不改进程当前目录
	fosPickFolders     = 0x00000020 // 只选文件夹
	fosForceFileSystem = 0x00000040 // 只显示文件系统里的目录
	fosAllMultiSelect  = 0x00000100 // 允许多选

	sigdnFileSysPath = 0x80058000 // SIGDN_FILESYSPATH：取文件系统路径

	// HRESULT_FROM_WIN32(ERROR_CANCELLED)：用户点了取消。
	hresultCancelled = 0x800704C7
	// RPC_E_CHANGED_MODE：线程已按别的 apartment 模式初始化过。
	rpcEChangedMode = 0x80010106

	// maxSelectedFiles 是多选结果的合理上限，用来兜住「vtable 索引写错
	// 导致 GetCount 返回垃圾值」的情况。
	maxSelectedFiles = 100000
)

// IFileDialog / IFileOpenDialog / IFileSaveDialog / IShellItem /
// IShellItemArray 的 vtable 索引。
//
// 顺序取自 Windows SDK 的 ShObjIdl_core.idl（接口方法按声明顺序进虚表）：
// IUnknown 占 0..2（QueryInterface/AddRef/Release），IModalWindow::Show 是 3，
// IFileDialog 的方法从 4 起，IFileOpenDialog / IFileSaveDialog 在其后追加。
const (
	idxShow             = 3
	idxSetFileTypes     = 4
	idxSetFileTypeIndex = 5
	idxGetFileTypeIndex = 6
	idxSetOptions       = 9
	idxGetOptions       = 10
	idxSetFolder        = 12
	idxGetFolder        = 13
	idxSetFileName      = 15
	idxGetFileName      = 16
	idxSetTitle         = 17
	idxGetResult        = 20
	idxSetDefaultExt    = 22
	// idxGetResultsFallback 是 IFileOpenDialog::GetResults 的历史下标
	//（IFileDialog 的基类末尾是 ClearClientData 时，GetResults 紧随其后）。
	// 新版 SDK 把 IFileDialog::SetFilter 并进了基类，GetResults 顺延一位，
	// 实际下标由 getResultsSlot 在运行时探测（见该函数）。
	idxGetResultsFallback = 26
	idxRelease            = 2 // IUnknown::Release
	idxShellGetDispName   = 5 // IShellItem::GetDisplayName
	idxArrayGetCount      = 7 // IShellItemArray::GetCount
	idxArrayGetItemAt     = 8 // IShellItemArray::GetItemAt
)

// comDlgFilterSpec 对应 COMDLG_FILTERSPEC：显示名 + 扩展名模式。
type comDlgFilterSpec struct {
	name    *uint16
	pattern *uint16
}

// dialogConfig 是一次文件对话框调用的参数。
type dialogConfig struct {
	title    string       // 对话框标题
	dir      string       // 初始目录
	fileName string       // 初始文件名（另存为）
	filters  []FileFilter // 文件类型过滤
	multi    bool         // 允许多选
	folders  bool         // 选目录而非文件
	save     bool         // 另存为（写模式）
}

// OpenFileDialog 弹出「打开文件」对话框。
//
// 返回用户选中的文件路径（多选时返回多个，顺序同对话框里的选择顺序）。
// 用户取消返回 (nil, nil)——取消不是错误。没有可用窗口（如无头模式）
// 返回错误。
func (win *Window) OpenFileDialog(title, dir string, filters []FileFilter, multi bool) ([]string, error) {
	parent, err := win.ownerWindow()
	if err != nil {
		return nil, err
	}
	return showFileDialog(parent, dialogConfig{
		title:   title,
		dir:     dir,
		filters: filters,
		multi:   multi,
	})
}

// OpenDirectoryDialog 弹出「选择文件夹」对话框，返回用户选中的目录。
// 用户取消返回 ("", nil)。
func (win *Window) OpenDirectoryDialog(title, dir string) (string, error) {
	parent, err := win.ownerWindow()
	if err != nil {
		return "", err
	}
	paths, err := showFileDialog(parent, dialogConfig{
		title:   title,
		dir:     dir,
		folders: true,
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

// SaveFileDialog 弹出「另存为」对话框，返回用户输入的保存路径。
// 用户取消返回 ("", nil)。
func (win *Window) SaveFileDialog(title, dir, name string, filters []FileFilter) (string, error) {
	parent, err := win.ownerWindow()
	if err != nil {
		return "", err
	}
	paths, err := showFileDialog(parent, dialogConfig{
		title:    title,
		dir:      dir,
		fileName: name,
		filters:  filters,
		save:     true,
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

// ownerWindow 返回窗口的 HWND（作为对话框的父窗口）。
//
// SDL_CreateWindow 返回的是 SDL_Window*，不是 HWND——原生对话框要拿真正的
// HWND 当父窗口（否则 Show 会回 E_INVALIDARG），只能问 SDL_GetWindowWMInfo。
// 取不到时返回 0：对话框照样能弹，只是不再挂在主窗口下面。
func (win *Window) ownerWindow() (uintptr, error) {
	if win == nil || win.closed || win.win == 0 {
		return 0, fmt.Errorf("窗口尚未创建，无法弹出文件对话框")
	}
	return win.hwnd(), nil
}

// sdlSysWMinfo 对应 SDL_syswm.h 的 SDL_SysWMinfo（Windows 分支）：
// version（SDL_version，3 字节 + 1 字节填充）、subsystem，以及 info
// 联合体里的 HWND / HDC / HINSTANCE。
type sdlSysWMinfo struct {
	version   [3]uint8
	_         [1]byte
	subsystem uint32
	winWindow uintptr
	winHDC    uintptr
	winHInst  uintptr
}

// hwnd 取窗口的原生 HWND；取不到返回 0。
func (win *Window) hwnd() uintptr {
	if !hasGetWindowWMInfo || sdlGetWindowWMInfo == nil {
		return 0
	}
	var info sdlSysWMinfo
	// info.version 必须按 SDL_VERSION 宏填成运行库的版本号，否则
	// SDL_GetWindowWMInfo 直接返回 SDL_FALSE。
	var ver [3]uint8
	sdlGetVersion(uintptr(unsafe.Pointer(&ver[0])))
	runtime.KeepAlive(&ver[0])
	info.version = ver
	if sdlGetWindowWMInfo(win.win, uintptr(unsafe.Pointer(&info))) == 0 {
		return 0
	}
	runtime.KeepAlive(&info)
	return info.winWindow
}

// showFileDialog 是三种对话框的公共流程：创建 → 配置 → Show → 取结果。
func showFileDialog(parent uintptr, cfg dialogConfig) ([]string, error) {
	// IFileDialog 必须在 STA 上创建与 Show。
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cleanup, err := comInit()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	dlg, err := createFileDialog(cfg)
	if err != nil {
		return nil, err
	}
	defer comRelease(dlg)

	// Show 是模态的：自带消息循环，阻塞当前线程直到用户关闭对话框。
	hr, _, _ := syscall.SyscallN(comMethod(dlg, idxShow), uintptr(dlg), parent)
	if uint32(hr) == hresultCancelled {
		return nil, nil // 用户取消
	}
	if err := hresultErr(hr); err != nil {
		return nil, fmt.Errorf("显示文件对话框失败: %w", err)
	}

	if cfg.multi {
		var arr unsafe.Pointer
		if err := comCall(comMethod(dlg, getResultsSlot()), dlg, uintptr(unsafe.Pointer(&arr))); err != nil {
			return nil, fmt.Errorf("读取选中项失败: %w", err)
		}
		defer comRelease(arr)
		return shellItemArrayPaths(arr)
	}

	var item unsafe.Pointer
	if err := comCall(comMethod(dlg, idxGetResult), dlg, uintptr(unsafe.Pointer(&item))); err != nil {
		return nil, fmt.Errorf("读取选中项失败: %w", err)
	}
	defer comRelease(item)
	path, err := shellItemPath(item)
	if err != nil {
		return nil, err
	}
	return []string{path}, nil
}

// createFileDialog 创建并配置一个文件对话框对象（尚未 Show）。
//
// 单独拆出来是为了能在无界面环境里验证「CLSID/IID 正确、vtable 索引
// 没写错」——Show 之后的路径需要真人操作，测不到。
func createFileDialog(cfg dialogConfig) (unsafe.Pointer, error) {
	clsid, iid := clsidFileOpenDialog, iidFileOpenDialog
	if cfg.save {
		clsid, iid = clsidFileSaveDialog, iidFileSaveDialog
	}
	var dlg unsafe.Pointer
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsid)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&dlg)))
	if err := hresultErr(hr); err != nil {
		return nil, fmt.Errorf("创建文件对话框失败: %w", err)
	}
	if dlg == nil {
		return nil, fmt.Errorf("创建文件对话框失败: 未取得接口指针")
	}
	if err := configureFileDialog(dlg, cfg); err != nil {
		comRelease(dlg)
		return nil, err
	}
	return dlg, nil
}

// configureFileDialog 设置对话框的标题、选项、过滤器、初始目录与文件名。
func configureFileDialog(dlg unsafe.Pointer, cfg dialogConfig) error {
	if cfg.title != "" {
		p, err := windows.UTF16PtrFromString(cfg.title)
		if err != nil {
			return fmt.Errorf("对话框标题含非法字符: %w", err)
		}
		if err := comCall(comMethod(dlg, idxSetTitle), dlg, uintptr(unsafe.Pointer(p))); err != nil {
			return fmt.Errorf("设置对话框标题失败: %w", err)
		}
		runtime.KeepAlive(p)
	}

	// 选项：始终不改进程当前目录；选目录模式只显示文件系统里的目录。
	opts := uint32(fosNoChangeDir | fosOverwritePrompt)
	if cfg.folders {
		opts |= fosPickFolders | fosForceFileSystem
	}
	if cfg.multi {
		opts |= fosAllMultiSelect
	}
	if err := comCall(comMethod(dlg, idxSetOptions), dlg, uintptr(opts)); err != nil {
		return fmt.Errorf("设置对话框选项失败: %w", err)
	}

	if specs := buildFilterSpecs(cfg.filters); len(specs) > 0 {
		if err := comCall(comMethod(dlg, idxSetFileTypes), dlg,
			uintptr(len(specs)), uintptr(unsafe.Pointer(&specs[0]))); err != nil {
			return fmt.Errorf("设置文件类型失败: %w", err)
		}
		runtime.KeepAlive(specs)
		// 默认选中第一个过滤器（与资源管理器一致）。
		if err := comCall(comMethod(dlg, idxSetFileTypeIndex), dlg, 1); err != nil {
			return fmt.Errorf("设置默认文件类型失败: %w", err)
		}
		// 另存为时按首个过滤器补默认扩展名（用户没填扩展名也会落到 .txt）。
		if cfg.save {
			if ext := defaultExtension(cfg.filters); ext != "" {
				p, err := windows.UTF16PtrFromString(ext)
				if err == nil {
					_ = comCall(comMethod(dlg, idxSetDefaultExt), dlg, uintptr(unsafe.Pointer(p)))
					runtime.KeepAlive(p)
				}
			}
		}
	}

	if cfg.fileName != "" {
		p, err := windows.UTF16PtrFromString(cfg.fileName)
		if err == nil {
			_ = comCall(comMethod(dlg, idxSetFileName), dlg, uintptr(unsafe.Pointer(p)))
			runtime.KeepAlive(p)
		}
	}

	if cfg.dir != "" {
		if err := setDialogFolder(dlg, cfg.dir); err != nil {
			return err
		}
	}
	return nil
}

// buildFilterSpecs 把过滤器转成 COMDLG_FILTERSPEC 数组。
//
// 宽串不能有内嵌 NUL，含 NUL 的过滤器直接跳过——一个坏过滤器不该让
// 整个对话框打不开。
func buildFilterSpecs(filters []FileFilter) []comDlgFilterSpec {
	specs := make([]comDlgFilterSpec, 0, len(filters))
	for _, f := range filters {
		if f.Name == "" || f.Pattern == "" {
			continue
		}
		name, err := windows.UTF16PtrFromString(f.Name)
		if err != nil {
			continue
		}
		pat, err := windows.UTF16PtrFromString(f.Pattern)
		if err != nil {
			continue
		}
		specs = append(specs, comDlgFilterSpec{name: name, pattern: pat})
	}
	return specs
}

// defaultExtension 从首个过滤器里取默认扩展名（"*.txt" → "txt"）。
//
// "*.*"（所有文件）没有默认扩展名，返回空串。
func defaultExtension(filters []FileFilter) string {
	for _, f := range filters {
		p := strings.TrimSpace(f.Pattern)
		if i := strings.Index(p, ";"); i >= 0 {
			p = strings.TrimSpace(p[:i])
		}
		if !strings.HasPrefix(p, "*.") {
			continue
		}
		ext := p[2:]
		if ext == "*" { // "*.*" = 所有文件
			return ""
		}
		return ext
	}
	return ""
}

// setDialogFolder 把对话框的初始目录设到 dir。
//
// 目录不存在时忽略错误：让对话框落在系统默认位置（通常是上次用过的
// 目录），而不是让「打开」整个不可用。
func setDialogFolder(dlg unsafe.Pointer, dir string) error {
	item, err := shellItemFromPath(dir)
	if err != nil || item == nil {
		return nil
	}
	defer comRelease(item)
	if err := comCall(comMethod(dlg, idxSetFolder), dlg, uintptr(item)); err != nil {
		return fmt.Errorf("设置初始目录失败: %w", err)
	}
	return nil
}

// shellItemFromPath 把路径解析成 shell item（IShellItem）。
//
// 路径不存在时返回 (nil, nil)——调用方按「没有这个路径」处理。
func shellItemFromPath(path string) (unsafe.Pointer, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, nil
	}
	var item unsafe.Pointer
	hr, _, _ := procSHCreateItemFromParsingName.Call(
		uintptr(unsafe.Pointer(p)), 0,
		uintptr(unsafe.Pointer(&iidShellItem)),
		uintptr(unsafe.Pointer(&item)))
	runtime.KeepAlive(p)
	if hresultErr(hr) != nil {
		return nil, nil
	}
	return item, nil
}

// shellItemPath 取 IShellItem 的文件系统路径。
func shellItemPath(item unsafe.Pointer) (string, error) {
	var name unsafe.Pointer
	if err := comCall(comMethod(item, idxShellGetDispName), item,
		sigdnFileSysPath, uintptr(unsafe.Pointer(&name))); err != nil {
		return "", fmt.Errorf("读取路径失败: %w", err)
	}
	if name == nil {
		return "", nil
	}
	// GetDisplayName 返回的是 COM 分配的内存，必须用 CoTaskMemFree 释放。
	defer procCoTaskMemFree.Call(uintptr(name))
	return windows.UTF16PtrToString((*uint16)(name)), nil
}

// shellItemArrayPaths 取出 IShellItemArray 里的全部路径（多选结果）。
func shellItemArrayPaths(arr unsafe.Pointer) ([]string, error) {
	var count uint32
	if err := comCall(comMethod(arr, idxArrayGetCount), arr, uintptr(unsafe.Pointer(&count))); err != nil {
		return nil, fmt.Errorf("读取选中数量失败: %w", err)
	}
	// 数量异常大说明 vtable 索引可能对错了（调到了别的方法）：直接报错，
	// 不去按这个数字开循环，避免把进程带崩。
	if count > maxSelectedFiles {
		return nil, fmt.Errorf("读取选中数量异常: %d", count)
	}
	paths := make([]string, 0, count)
	for i := uint32(0); i < count; i++ {
		var item unsafe.Pointer
		if err := comCall(comMethod(arr, idxArrayGetItemAt), arr,
			uintptr(i), uintptr(unsafe.Pointer(&item))); err != nil {
			return paths, fmt.Errorf("读取第 %d 个选中项失败: %w", i, err)
		}
		path, err := shellItemPath(item)
		comRelease(item)
		if err != nil {
			return paths, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// comInit 把当前线程初始化成 STA（IFileDialog 必须在 STA 上创建与 Show）。
//
// 返回的清理函数要在对话框用完后调用。线程若已按别的 apartment 模式
// 初始化（RPC_E_CHANGED_MODE），Shell 对话框仍可用，只是不能反初始化。
func comInit() (func(), error) {
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	switch int32(hr) {
	case 0, 1: // S_OK / S_FALSE：本次初始化成功，需要配对 CoUninitialize
		return func() { procCoUninitialize.Call() }, nil
	default:
		if uint32(hr) == rpcEChangedMode {
			return func() {}, nil
		}
		return nil, fmt.Errorf("CoInitializeEx 失败: HRESULT 0x%08X", uint32(hr))
	}
}

// comMethod 取 COM 对象虚表里的第 index 个方法地址。
//
// obj 是接口指针：虚表首指针就放在对象内存的第一个字段，因此按
// *(**uintptr) 读出，再按方法下标取地址。索引以 ShObjIdl_core.idl 为准。
func comMethod(obj unsafe.Pointer, index int) uintptr {
	vt := *(**uintptr)(obj)
	return *(*uintptr)(unsafe.Add(unsafe.Pointer(vt), uintptr(index)*unsafe.Sizeof(uintptr(0))))
}

// comCall 调用 COM 方法：fn 为虚表里的方法地址，obj 为 this 指针。
func comCall(fn uintptr, obj unsafe.Pointer, args ...uintptr) error {
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, uintptr(obj))
	all = append(all, args...)
	hr, _, _ := syscall.SyscallN(fn, all...)
	return hresultErr(hr)
}

// comRelease 释放一个 COM 引用。
func comRelease(obj unsafe.Pointer) {
	if obj == nil {
		return
	}
	syscall.SyscallN(comMethod(obj, idxRelease), uintptr(obj))
}

// hresultErr 把失败的 HRESULT 转成 error；成功返回 nil。
func hresultErr(hr uintptr) error {
	if int32(hr) >= 0 {
		return nil
	}
	return fmt.Errorf("HRESULT 0x%08X", uint32(hr))
}

// probeFilterObject 是只实现 IUnknown 的最小 COM 对象，用来探测 vtable 布局。
//
// 它的虚表只有 QueryInterface/AddRef/Release 三个槽：QueryInterface 对任何
// IID 都返回自身，AddRef/Release 返回 1。它只用于「塞给候选槽看看对方
// 会不会改这块内存」，绝不能被真的调用到第 4 个槽（越界）。
type probeFilterObject struct {
	vtbl *[3]uintptr
}

// newProbeFilterObject 造一个探测用的假 COM 对象。
func newProbeFilterObject() *probeFilterObject {
	// QueryInterface 一律回 E_NOINTERFACE：探测对象不实现任何真实接口，
	// 免得 Shell 拿到半成品的接口指针去调不存在的槽位。
	qi := windows.NewCallback(func(this, riid, ppv, _ uintptr) uintptr {
		return 0x80004002 // E_NOINTERFACE
	})
	addRef := windows.NewCallback(func(this, _, _, _ uintptr) uintptr { return 1 })
	release := windows.NewCallback(func(this, _, _, _ uintptr) uintptr { return 1 })
	return &probeFilterObject{vtbl: &[3]uintptr{qi, addRef, release}}
}

// getResultsSlot 返回 IFileOpenDialog::GetResults 的 vtable 下标（进程内只探测一次）。
//
// 两种布局都真实存在：
//   - 旧（Windows 7 ~ Windows 11 23H2）：IFileDialog 的方法到 ClearClientData
//     为止，IFileOpenDialog::GetResults 落在 26；
//   - 新（后来的 SDK 把 IFileDialog::SetFilter 并进基类）：26 变成
//     SetFilter，GetResults 顺延到 27。
//
// 固定写死任何一个都会在另一种系统上崩（调到相邻方法），所以运行时探测。
// 探测方法：给候选槽传一个只实现 IUnknown 的假 COM 对象——出参方法
// （GetResults）会把结果指针写进这块内存（假对象的虚表指针被覆盖），
// 入参方法（SetFilter）只会 AddRef 它，内存不变。
//
// 探测在一个用完即弃的对话框对象上做：SetFilter 会把假过滤器装到对话框上，
// 真拿去用会调到假对象里不存在的槽位。
var (
	getResultsOnce sync.Once
	getResultsIdx  = idxGetResultsFallback
)

func getResultsSlot() int {
	getResultsOnce.Do(func() {
		if slot, ok := detectGetResultsSlot(); ok {
			getResultsIdx = slot
		}
	})
	return getResultsIdx
}

// detectGetResultsSlot 探测 GetResults 的下标；探测不出来时返回 false。
func detectGetResultsSlot() (int, bool) {
	cleanup, err := comInit()
	if err != nil {
		return 0, false
	}
	defer cleanup()

	dlg, err := createFileDialog(dialogConfig{title: "probe"})
	if err != nil {
		return 0, false
	}
	defer comRelease(dlg)

	for slot := idxGetResultsFallback; slot <= idxGetResultsFallback+2; slot++ {
		fake := newProbeFilterObject()
		before := fake.vtbl
		syscall.SyscallN(comMethod(dlg, slot), uintptr(dlg), uintptr(unsafe.Pointer(fake)))
		if fake.vtbl != before {
			return slot, true // 出参方法：结果指针写进了我们给的内存
		}
	}
	return 0, false
}
