package engine

import (
	"reflect"
	"testing"
)

// containsPointer 判断类型的值表示里是否含需要 GC 追踪的引用（接口/指针/
// 切片/map/string/func）。数组与结构体递归查看成员。
func containsPointer(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice,
		reflect.String, reflect.Func, reflect.UnsafePointer:
		return true
	case reflect.Array:
		return containsPointer(t.Elem())
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if containsPointer(t.Field(i).Type) {
				return true
			}
		}
	}
	return false
}

// layoutHoles 返回结构体字段之间的对齐空洞总字节数（含尾部填充）。
func layoutHoles(typ reflect.Type) (holes int, where string) {
	var end uintptr
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if i > 0 && f.Offset > end {
			holes += int(f.Offset - end)
			where = typ.Field(i - 1).Name
		}
		end = f.Offset + f.Type.Size()
	}
	holes += int(typ.Size() - end)
	return
}

// 节点结构体与样式结构体每个文档节点各有一份，字段顺序直接影响常驻内存：
// 含指针的字段必须全部排在前面（GC 的指针扫描按 ptrdata 收尾），bool 与
// 单字节枚举必须聚在一起放在尾部（夹在 8 字节字段之间会各自撑出 5~7B 空洞）。
// 这两条一旦破功，六万节点的页面就要多付近 6MB，故在此钉住：
//
//	htmlElement         624B（重排前 680B）
//	cssStyleDeclaration 400B（重排前 440B）
func TestNodeStructFieldPacking(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(htmlElement{}), reflect.TypeOf(cssStyleDeclaration{})} {
		name := typ.Name()
		want := map[string]int{"htmlElement": 624, "cssStyleDeclaration": 400}[name]
		if got := int(typ.Size()); got != want {
			t.Errorf("%s sizeof = %d，期望 %d（字段分段被打破？新增字段请按「含指针 → 纯数值 → 单字节标记」归位）",
				name, got, want)
		}
		// 含指针的字段不得出现在纯数字段之后。
		sawPlain := ""
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if containsPointer(f.Type) {
				if sawPlain != "" {
					t.Errorf("%s.%s 含指针却排在无指针字段 %s 之后，会把 GC 扫描范围拉满整个结构体",
						name, f.Name, sawPlain)
					break
				}
			} else {
				sawPlain = f.Name
			}
		}
		// 空洞只允许是尾部对齐填充（≤ 一个机器字）。
		if holes, at := layoutHoles(typ); holes > 8 {
			t.Errorf("%s 字段间空洞 %dB（最后出现在 %s 之后），单字节字段被夹进了 8 字节字段之间",
				name, holes, at)
		}
	}
}
