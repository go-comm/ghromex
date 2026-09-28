package engine

// DefineCustomElement 注册自定义标签原型（类比 Web Components 的 customElements.define）。
// 原型须由 newElement(tag) 起步构建；重复 Define 同一标签会直接返回错误而不覆盖。
func DefineCustomElement(tag string, prototype HTMLElement) error {
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
