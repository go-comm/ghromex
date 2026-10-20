package components

import "github.com/go-comm/ghromex/engine"

var defaultClassesTemplate = `

.btn {
	margin:5px 0;
	padding:15px 8px;
	border:1px solid #C8C8C8;
	border-radius: 4px 4px;
}

.btn-group {
	margin:5px 0;
	padding:15px 8px;
	border:1px solid #C8C8C8;
	border-radius: 4px 4px;
	overflow: hidden;
}

.btn-group > .btn {
	margin:0 0;
}

.input {
	margin:5px 0;
	padding:15px 8px;
	border:1px solid #333;
	border-radius: 4px;
}

`

// InjectDefaultClasses 把默认类样式（.btn/.btn-group/.input）注入文档。
// 此前 defaultClassesTemplate 从未被任何地方使用——组件工厂产出的元素
// （CreateButton 挂 class="btn"、CreateInput 挂 class="input"）拿不到对应
// 样式，README 宣称的"class 工具"实为空操作。
func InjectDefaultClasses(doc engine.HTMLDocument) {
	if doc == nil {
		return
	}
	doc.AddStylesheetText(defaultClassesTemplate)
}
