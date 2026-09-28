package engine

type HTMLCollection interface {
	Item(i int) HTMLElement
	NamedItem(name string) HTMLElement
	Length() int
}
