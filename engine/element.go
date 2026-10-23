package engine

type ELement interface {
	SetAttribute(key string, value string)
	// RemoveAttribute 删除属性；不存在时是空操作。
	//
	// 布尔属性（disabled/checked 等）按「属性是否存在」判定，启用控件时
	// 不能只把值改成空串，必须真正删掉属性。
	RemoveAttribute(key string)
	GetAttribute(key string) string
	ID() string
	SetID(id string)
}
