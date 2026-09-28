package engine

import "sync"

var (
	globalSystemElements = &elementRegistry{}
	globalCustomElements = &elementRegistry{}
)

func SystemElements() *elementRegistry {
	return globalSystemElements
}

func CustomElements() *elementRegistry {
	return globalCustomElements
}

// elementRegistry 保存标签原型。Get 返回的是原型的引用（仅供读取 TagName 等），
// 创建新元素必须走 CloneElement，避免同名元素共享状态。
type elementRegistry struct {
	elements sync.Map
}

func (registry *elementRegistry) Define(tag string, element HTMLElement) {
	registry.elements.Store(tag, element)
}

func (registry *elementRegistry) Get(tag string) HTMLElement {
	o, _ := registry.elements.Load(tag)
	if o != nil {
		return o.(HTMLElement)
	}
	return nil
}

// CloneElement 克隆注册表中的原型，返回全新的独立实例。
func CloneElement(e HTMLElement) HTMLElement {
	if e == nil {
		return nil
	}
	return e.Clone()
}
