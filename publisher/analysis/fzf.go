package analysis

import (
	"fmt"

	"github.com/btidor/src.codes/fzf"
)

func ConstructFzfIndex(a Archive) fzf.Node {
	return fzfIndexDirectory(a.Pkg.Name, a.Tree)
}

func fzfIndexDirectory(name string, dir Directory) fzf.Node {
	var node = fzf.Node{
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
