package fzf

import "github.com/vmihailenco/msgpack/v5"

type Node struct {
	//lint:ignore U1000 msgpack config options
	_msgpack struct{} `msgpack:",as_array"`

	Name     []byte
	Files    [][]byte
	Children []Node
}

var _ msgpack.CustomDecoder = (*Node)(nil)

func (n *Node) DecodeMsgpack(d *msgpack.Decoder) error {
	_, err := d.DecodeArrayLen()
	if err != nil {
		return err
	}

	var name string
	var files []string
	err = d.DecodeMulti(&name, &files, &n.Children)
	if err != nil {
		return err
	}

	// TODO: this makes it impossible to open files containing non-ASCII chars,
	// since the filename does not match the filesystem path
	n.Name = []byte{'/'}
	for _, r := range name {
		if r < 128 {
			n.Name = append(n.Name, byte(r))
		} else {
			n.Name = append(n.Name, byte(0))
		}
	}
	n.Files = make([][]byte, len(files))
	for i, f := range files {
		n.Files[i] = []byte{'/'}
		for _, r := range f {
			if r < 128 {
				n.Files[i] = append(n.Files[i], byte(r))
			} else {
				n.Files[i] = append(n.Files[i], byte(0))
			}
		}
	}
	return nil
}
