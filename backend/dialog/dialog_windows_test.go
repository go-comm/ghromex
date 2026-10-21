//go:build windows

package dialog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// withCOM 在 STA 上初始化 COM 并跑 fn，结束后反初始化。
//
// IFileDialog 之类的 COM 对象必须在初始化过的线程上创建：没初始化时
// CoCreateInstance 直接回 CO_E_NOTINITIALIZED（0x800401F0）。COM 的
// 初始化/反初始化必须落在同一个 OS 线程上，所以这里锁线程——Go 的
// 协程默认会在多个线程间迁移。
func withCOM(t *testing.T, fn func()) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cleanup, err := comInit()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	fn()
}

// TestCreateFileDialog 校验「CLSID/IID 正确、vtable 索引没写错」。
//
// 创建一个文件对话框并配置好（标题/过滤器/初始目录/文件名）后释放：
// 配置阶段走的正是 Show 之前的那批 vtable 索引（SetTitle/SetOptions/
// SetFileTypes/SetFileTypeIndex/SetFileName/SetFolder），索引写错会直接
// 崩或报错。Show 之后的选择流程要真人操作，由 dialog_ui_test.go 覆盖。
func TestCreateFileDialog(t *testing.T) {
	withCOM(t, func() {
		dir := t.TempDir()
		dlg, err := createFileDialog(dialogConfig{
			title:    "打开文件",
			dir:      dir,
			fileName: "note.txt",
			filters: []FileFilter{
				{Name: "文本文件 (*.txt)", Pattern: "*.txt"},
				{Name: "所有文件 (*.*)", Pattern: "*.*"},
			},
		})
		if err != nil {
			t.Fatalf("创建打开对话框失败: %v", err)
		}
		comRelease(dlg)
	})
}

// 另存为走的是另一组 CLSID/IID（IFileSaveDialog），单独校验一次。
func TestCreateSaveFileDialog(t *testing.T) {
	withCOM(t, func() {
		dlg, err := createFileDialog(dialogConfig{
			title:   "另存为",
			dir:     t.TempDir(),
			save:    true,
			filters: []FileFilter{{Name: "文本文件 (*.txt)", Pattern: "*.txt"}},
		})
		if err != nil {
			t.Fatalf("创建另存为对话框失败: %v", err)
		}
		comRelease(dlg)
	})
}

// 选目录：同一套接口，换一组选项（FOS_PICKFOLDERS）。
func TestCreateDirectoryDialog(t *testing.T) {
	withCOM(t, func() {
		dlg, err := createFileDialog(dialogConfig{
			title:   "选择文件夹",
			dir:     t.TempDir(),
			folders: true,
		})
		if err != nil {
			t.Fatalf("创建目录对话框失败: %v", err)
		}
		comRelease(dlg)
	})
}

// 初始目录不存在时不能让整个对话框失败：应当退回系统默认位置。
func TestCreateDialogIgnoresMissingDir(t *testing.T) {
	withCOM(t, func() {
		dlg, err := createFileDialog(dialogConfig{
			title: "打开文件",
			dir:   filepath.Join(t.TempDir(), "no-such-dir"),
		})
		if err != nil {
			t.Fatalf("初始目录不存在时创建对话框失败: %v", err)
		}
		comRelease(dlg)
	})
}

// TestDialogConfigRoundTrip 配置项要能被读回来：SetFileTypes/SetOptions/
// SetFileName/SetFolder 各自有对应的 getter，读回来的值与写进去的一致，
// 说明这四个下标没有错位（错位会读到相邻方法，读出乱值或直接崩）。
func TestDialogConfigRoundTrip(t *testing.T) {
	withCOM(t, func() {
		dir := t.TempDir()
		dlg, err := createFileDialog(dialogConfig{
			title:    "打开文件",
			dir:      dir,
			fileName: "note.txt",
			filters:  []FileFilter{{Name: "文本文件 (*.txt)", Pattern: "*.txt"}},
		})
		if err != nil {
			t.Fatalf("创建对话框失败: %v", err)
		}
		defer comRelease(dlg)

		var typeIndex uint32
		if err := comCall(comMethod(dlg, idxGetFileTypeIndex), dlg, uintptr(unsafe.Pointer(&typeIndex))); err != nil {
			t.Fatalf("读文件类型下标失败: %v", err)
		}
		if typeIndex != 1 {
			t.Errorf("默认文件类型下标 = %d, want 1", typeIndex)
		}

		var opts uint32
		if err := comCall(comMethod(dlg, idxGetOptions), dlg, uintptr(unsafe.Pointer(&opts))); err != nil {
			t.Fatalf("读对话框选项失败: %v", err)
		}
		if opts&fosNoChangeDir == 0 {
			t.Errorf("对话框选项 = 0x%X, want 含 FOS_NO_CHANGEDIR", opts)
		}
		if opts&fosOverwritePrompt == 0 {
			t.Errorf("对话框选项 = 0x%X, want 含 FOS_OVERWRITEPROMPT", opts)
		}

		var namePtr unsafe.Pointer
		if err := comCall(comMethod(dlg, idxGetFileName), dlg, uintptr(unsafe.Pointer(&namePtr))); err != nil {
			t.Fatalf("读文件名失败: %v", err)
		}
		if namePtr != nil {
			defer procCoTaskMemFree.Call(uintptr(namePtr))
		}
		if got := windowsUTF16(namePtr); got != "note.txt" {
			t.Errorf("读回的文件名 = %q, want %q", got, "note.txt")
		}

		var folder unsafe.Pointer
		if err := comCall(comMethod(dlg, idxGetFolder), dlg, uintptr(unsafe.Pointer(&folder))); err != nil {
			t.Fatalf("读初始目录失败: %v", err)
		}
		if folder == nil {
			t.Fatalf("初始目录没有被记下")
		}
		defer comRelease(folder)
		got, err := shellItemPath(folder)
		if err != nil {
			t.Fatalf("读初始目录路径失败: %v", err)
		}
		if !samePath(got, dir) {
			t.Errorf("初始目录 = %q, want %q", got, dir)
		}
	})
}

// TestGetResultsSlotDetection 多选结果的入口（IFileOpenDialog::GetResults）
// 在不同 Windows 版本上落在不同的虚表槽位，实现是运行时探测的——这里
// 反过来验一次：探测到的槽位必须是一个「出参」方法（会往我们给的内存里
// 写结果指针）。
func TestGetResultsSlotDetection(t *testing.T) {
	withCOM(t, func() {
		dlg, err := createFileDialog(dialogConfig{title: "probe"})
		if err != nil {
			t.Fatalf("创建对话框失败: %v", err)
		}
		defer comRelease(dlg)

		slot := getResultsSlot()
		if slot < idxGetResultsFallback || slot > idxGetResultsFallback+2 {
			t.Fatalf("探测到的下标 = %d，超出候选范围", slot)
		}
		fake := newProbeFilterObject()
		before := fake.vtbl
		syscall.SyscallN(comMethod(dlg, slot), uintptr(dlg), uintptr(unsafe.Pointer(fake)))
		if fake.vtbl == before {
			t.Errorf("下标 %d 不是出参方法（GetResults 应把结果指针写进这块内存）", slot)
		}
	})
}

// TestShellItemPath 校验 IShellItem::GetDisplayName 的 vtable 索引：
// 把目录解析成 shell item，再取回它的文件系统路径。
func TestShellItemPath(t *testing.T) {
	withCOM(t, func() {
		dir := t.TempDir()
		item, err := shellItemFromPath(dir)
		if err != nil {
			t.Fatalf("解析目录失败: %v", err)
		}
		if item == nil {
			t.Fatalf("目录 %q 没有解析出 shell item", dir)
		}
		defer comRelease(item)

		got, err := shellItemPath(item)
		if err != nil {
			t.Fatalf("读取路径失败: %v", err)
		}
		if !samePath(got, dir) {
			t.Errorf("取回的路径 = %q, want %q", got, dir)
		}
	})
}

// 不存在的路径解析不出 shell item（调用方据此忽略错误）。
func TestShellItemFromMissingPath(t *testing.T) {
	withCOM(t, func() {
		item, err := shellItemFromPath(filepath.Join(t.TempDir(), "missing.txt"))
		if err != nil {
			t.Fatalf("解析不存在的路径报错: %v", err)
		}
		if item != nil {
			comRelease(item)
			t.Errorf("不存在的路径解析出了 shell item")
		}
	})
}

// 过滤器转换：宽串不能有内嵌 NUL，坏的过滤器直接跳过。
func TestBuildFilterSpecs(t *testing.T) {
	specs := buildFilterSpecs([]FileFilter{
		{Name: "文本文件 (*.txt)", Pattern: "*.txt"},
		{Name: "坏名字", Pattern: "*.txt\x00"},
		{Name: "", Pattern: "*.md"},
	})
	if len(specs) != 1 {
		t.Fatalf("有效过滤器数 = %d, want 1", len(specs))
	}
	if specs[0].name == nil || specs[0].pattern == nil {
		t.Fatalf("过滤器宽串为空: %+v", specs[0])
	}
}

// 默认扩展名取自首个过滤器（"*.txt" → "txt"；"*.*" 没有默认扩展名）。
func TestDefaultExtension(t *testing.T) {
	cases := []struct {
		filters []FileFilter
		want    string
	}{
		{[]FileFilter{{Name: "文本", Pattern: "*.txt"}}, "txt"},
		{[]FileFilter{{Name: "文档", Pattern: "*.md;*.markdown"}}, "md"},
		{[]FileFilter{{Name: "全部", Pattern: "*.*"}}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := defaultExtension(c.filters); got != c.want {
			t.Errorf("defaultExtension(%v) = %q, want %q", c.filters, got, c.want)
		}
	}
}

// 没有父窗口时对话框仍可弹出（parent=0 表示不挂父窗口）——但不写这条
// 测试：它会真的弹出模态对话框，无头/CI 环境会一直等到超时。
// 「无窗口时应用层拿到 ErrUnsupported」由 browser 门面的测试覆盖
// （browser.TestDialogWithoutWindow）。

// 多选对话框要真的置上 FOS_ALLOWMULTISELECT：标志位写错（例如误用
// 0x00000100 —— 那是 FOS_NOVALIDATE）时，多选会失效，而且顺带关掉了
// 文件存在性检查。
func TestMultiSelectOption(t *testing.T) {
	withCOM(t, func() {
		dlg, err := createFileDialog(dialogConfig{title: "打开文件", multi: true})
		if err != nil {
			t.Fatalf("创建多选对话框失败: %v", err)
		}
		defer comRelease(dlg)

		var opts uint32
		if err := comCall(comMethod(dlg, idxGetOptions), dlg, uintptr(unsafe.Pointer(&opts))); err != nil {
			t.Fatalf("读对话框选项失败: %v", err)
		}
		if opts&fosAllowMultiSelect == 0 {
			t.Errorf("多选对话框选项 = 0x%X, want 含 FOS_ALLOWMULTISELECT", opts)
		}
	})
}

// 单选对话框不能带多选标志（否则用户能勾多个，调用方只取一个）。
func TestSingleSelectHasNoMultiSelectFlag(t *testing.T) {
	withCOM(t, func() {
		dlg, err := createFileDialog(dialogConfig{title: "打开文件"})
		if err != nil {
			t.Fatalf("创建对话框失败: %v", err)
		}
		defer comRelease(dlg)

		var opts uint32
		if err := comCall(comMethod(dlg, idxGetOptions), dlg, uintptr(unsafe.Pointer(&opts))); err != nil {
			t.Fatalf("读对话框选项失败: %v", err)
		}
		if opts&fosAllowMultiSelect != 0 {
			t.Errorf("单选对话框选项 = 0x%X, want 不含 FOS_ALLOWMULTISELECT", opts)
		}
	})
}

// samePath 比较两个路径（Windows 上大小写不敏感，尾部斜杠可省略）。
func samePath(a, b string) bool {
	na := strings.TrimRight(strings.ToLower(a), `\/`)
	nb := strings.TrimRight(strings.ToLower(b), `\/`)
	return na == nb
}

// windowsUTF16 把 COM 返回的宽串指针转成 Go 字符串（空指针返回空串）。
func windowsUTF16(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	return windows.UTF16PtrToString((*uint16)(p))
}

// 确认测试用的临时目录确实存在（避免 samePath 误判）。
func TestSamePathHelper(t *testing.T) {
	dir := t.TempDir()
	if !samePath(dir, dir+`\`) {
		t.Errorf("尾部斜杠应视为同一路径")
	}
	if samePath(dir, filepath.Join(dir, "sub")) {
		t.Errorf("不同路径不应相等")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}
