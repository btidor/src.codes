package fzf

type Node struct {
	//lint:ignore U1000 msgpack config options
	_msgpack struct{} `msgpack:",as_array"`

	Name     string
	Files    []string
	Children []*Node
}
