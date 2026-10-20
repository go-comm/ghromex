package engine

import (
	"reflect"
	"sync"
)

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
	if o == nil {
		return nil
	}
	// 双返回值断言：注册表里若存了非 HTMLElement（外部 API 直接塞入），
	// 单值断言会 panic——这里降级为 nil。
	e, _ := o.(HTMLElement)
	return e
}

// CloneElement 克隆注册表中的原型，返回全新的独立实例。
// 原型是 typed nil（接口非 nil、指针为 nil）时返回 nil：调 Clone 会空指针崩溃。
func CloneElement(e HTMLElement) HTMLElement {
	if e == nil {
		return nil
	}
	if v := reflect.ValueOf(e); v.Kind() == reflect.Ptr && v.IsNil() {
		return nil
	}
	return e.Clone()
}
