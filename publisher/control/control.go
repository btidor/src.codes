// Package control provides functions for reading Debian's "control data"
// (*.dsc) format.
//
// https://www.debian.org/doc/debian-policy/ch-controlfields.html
//
package control

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Document map[string]string

type File struct {
	Name string
	Size uint64
	Hash string
}

func Parse(s string) (Document, error) {
	var d = make(Document)

	var lines = strings.Split(strings.TrimSpace(s), "\n")

	var key string
	var value strings.Builder

	for _, line := range lines {
		if len(line) == 0 {
			return nil, fmt.Errorf("unexpected blank line")
		}

		line = strings.TrimRight(line, "\r\n")
		r, _ := utf8.DecodeRuneInString(line)
		if unicode.IsSpace(r) {
			// Continuation of the previous entry
			if key == "" {
				return nil, fmt.Errorf("found continuation before first key")
			}
			value.WriteString(line)
		} else {
			// Finish previous entry
			if key != "" {
				d[key] = value.String()
			}

			// Start next entry
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("could not parse line: %#v", line)
			}
			key = parts[0]
			value = strings.Builder{}
			value.WriteString(strings.TrimLeftFunc(parts[1], unicode.IsSpace))
		}
	}

	// Finish final entry
	if key != "" {
		d[key] = value.String()
	}
	return d, nil
}

func (d Document) GetString(key string) string {
	if entry, found := (d)[key]; found {
		return entry
	} else {
		var keys []string
		for w := range d {
			keys = append(keys, w)
		}
		err := fmt.Errorf("missing key %#v\nfound: %#v", key, keys)
		panic(err)
	}
}

func (d Document) GetFiles(key string) []File {
	var raw = d.GetString(key)
	var fields = strings.Fields(raw)
	if len(fields)%3 != 0 {
		err := fmt.Errorf("not a multiple of 3: key %#v\nvalue %#v", key, raw)
		panic(err)
	}

	var files []File
	for i := 0; i < len(fields); i += 3 {
		size, err := strconv.ParseUint(fields[i+1], 10, 64)
		if err != nil {
			panic(err)
		}
		files = append(files, File{
			Name: fields[i+2],
			Size: size,
			Hash: fields[i],
		})
	}
	return files
}
func FindFileInList(files []File, name string) File {
	var match File
	var found bool
	for _, file := range files {
		if file.Name == name {
			if found {
				err := fmt.Errorf("duplicate matches for %#v: %#v, %#v",
					name, match.Name, file.Name,
				)
				panic(err)
			} else {
				match = file
				found = true
			}
		}
	}
	if !found {
		err := fmt.Errorf("no file found for %#v", name)
		panic(err)
	}
	return match
}
