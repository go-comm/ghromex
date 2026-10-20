package engine

import "reflect"

// DefineCustomElement 注册自定义标签原型（类比 Web Components 的 customElements.define）。
// 原型须由 newElement(tag) 起步构建；重复 Define 同一标签会直接返回错误而不覆盖。
func DefineCustomElement(tag string, prototype HTMLElement) error {
	if prototype == nil {
		return &engineError{msg: "custom element prototype is nil: " + tag}
	}
	// 「非 nil 接口 + nil 指针」（typed nil）同样危险：Clone 会调到提升自 nil
	// 指针的方法 → 空指针崩溃。注册期就拒掉，别留到 CreateElement 才炸。
	if v := reflect.ValueOf(prototype); v.Kind() == reflect.Ptr && v.IsNil() {
		return &engineError{msg: "custom element prototype is a typed nil pointer: " + tag}
	}
	if CustomElements().Get(tag) != nil {
		return &engineError{msg: "custom element already defined: " + tag}
	}
	CustomElements().Define(tag, prototype)
	return nil
}

type engineError struct {
	msg string
}

func (e *engineError) Error() string {
	return "ghromex: " + e.msg
}
