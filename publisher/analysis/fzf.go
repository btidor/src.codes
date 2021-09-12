package analysis

import (
	"fmt"
)

type FzfNode struct {
	//lint:ignore U1000 msgpack config options
	_msgpack struct{} `msgpack:",as_array"`

	Name     string
	Files    []string
	Children []*FzfNode
}

func ConstructFzfIndex(a Archive) FzfNode {
	return fzfIndexDirectory(a.Pkg.Name, a.Tree)
}

func fzfIndexDirectory(name string, dir Directory) FzfNode {
	var node = FzfNode{
		Name: name,
	}
	for name, value := range dir.Contents {
		switch value := value.(type) {
		case File:
			node.Files = append(node.Files, name)
		case Directory:
			dir := fzfIndexDirectory(name, value)
			node.Children = append(node.Children, &dir)
		case SymbolicLink:
		default:
			err := fmt.Errorf("unknown node type: %#v", node)
			panic(err)
		}
	}
	return node
}
