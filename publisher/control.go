package main

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ControlFile struct {
	Contents map[string]string
}

type ControlFileList struct {
	Files map[string]ControlFileEntry
}

type ControlFileEntry struct {
	Name string
	Size uint64
	Hash string
}

// ParseControlFile parses the Debian control file format used by the Sources
// index and "package.dsc" files. It returns two separate maps: one for
// single-line values (e.g. Date), and another for multi-line lists (e.g.
// Files).
func ParseControlFile(contents string) (*ControlFile, error) {
	result := make(map[string]string)
	var builder strings.Builder
	var currentKey string

	cleaned := strings.TrimSpace(contents)
	for _, line := range strings.Split(cleaned, "\n") {
		if len(line) == 0 {
			return nil, fmt.Errorf("unexpected blank line in control file")
		}

		line = strings.TrimRight(line, "\r\n")
		r, _ := utf8.DecodeRuneInString(line)
		if unicode.IsSpace(r) {
			// Continuation of the previous line
			builder.WriteString(line)
		} else {
			// Finish previous entry
			if currentKey != "" {
				result[currentKey] = builder.String()
			}

			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("could not parse line: %#v", line)
			}
			currentKey = parts[0]
			builder = strings.Builder{}
			builder.WriteString(strings.TrimLeftFunc(parts[1], unicode.IsSpace))
		}
	}

	// Finish final entry
	if currentKey != "" {
		result[currentKey] = builder.String()
	}
	return &ControlFile{Contents: result}, nil
}

func (c *ControlFile) MustGetStringEntry(key string) string {
	if entry, found := c.Contents[key]; found {
		return entry
	} else {
		err := fmt.Errorf("control file is missing key %#v\n\ncontents: %#v", key, c.Contents)
		panic(err)
	}
}

func (c *ControlFile) MustGetListEntry(key string) *ControlFileList {
	raw := c.MustGetStringEntry(key)
	fields := strings.Fields(raw)
	if len(fields)%3 != 0 {
		err := fmt.Errorf("control file entry %#v is not a length multiple of 3\n\nvalue: %#v", key, raw)
		panic(err)
	}

	result := make(map[string]ControlFileEntry)
	for i := 0; i < len(fields); i += 3 {
		filename := fields[i+2]
		size, err := strconv.ParseUint(fields[i+1], 10, 64)
		if err != nil {
			panic(err)
		}
		hash := fields[i]

		result[filename] = ControlFileEntry{Name: filename, Size: size, Hash: hash}
	}
	return &ControlFileList{Files: result}
}

func (c *ControlFileList) MustGetFile(pathComponents ...string) *ControlFileEntry {
	filename := path.Join(pathComponents...)
	file, found := c.Files[filename]
	if !found {
		err := fmt.Errorf("file not found: %#v\n\ncontents: %#v", filename, c)
		panic(err)
	}
	return &file
}

func (c *ControlFileList) FindFile(re *regexp.Regexp) (*ControlFileEntry, error) {
	var match ControlFileEntry
	var found bool
	for filename := range c.Files {
		if re.MatchString(filename) {
			if !found {
				match = c.Files[filename]
				found = true
			} else {
				return nil, fmt.Errorf("found multiple matches for file %#v e.g. %s, %s", re.String(), filename, match.Name)
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("no file found matching %#v", re.String())
	} else {
		return &match, nil
	}
}
