package engine

type ELement interface {
	SetAttribute(key string, value string)
	GetAttribute(key string) string
	ID() string
	SetID(id string)
}
