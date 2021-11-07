package analysis

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
)

// TODO: this regex can't handle nested angle brackets in C++ code; see exiv2
// for examples
var symbolExtractor = regexp.MustCompile(`^ ([^@]+::)?([A-Za-z0-9_~]+)(<[^@]+>)?@.*$`)

func ConstructSymbolsIndex(a Archive, ctags []byte) []byte {
	pattern := path.Join(a.Dir, "debian/*symbols")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		panic(err)
	}

	tagIndex := parseCtags(ctags)

	var result []byte
	for _, filename := range matches {
		header := fmt.Sprintf("### %s %s\n", a.Pkg.Name, filepath.Base(filename))
		result = append(result, []byte(header)...)

		sym := bytes.NewReader(
			processSymbols(filename),
		)
		sc := bufio.NewScanner(sym)
		for sc.Scan() {
			line := sc.Text()
			result = append(result, []byte(line)...)
			result = append(result, '\n')
			if matches := symbolExtractor.FindStringSubmatch(line); matches != nil {
				token := matches[2]
				if tags, ok := tagIndex[token]; ok {
					for _, tag := range tags {
						record := fmt.Sprintf(" - %s\n", tag)
						result = append(result, []byte(record)...)
					}
				}
			}
		}
		result = append(result, '\n')
	}
	return result
}

func processSymbols(filename string) []byte {
	// Exclude params for now, since they make it more difficult to extract the
	// function name and match it to the ctags index.
	cmd := exec.Command("c++filt", "--no-params")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}

	go func() {
		defer stdin.Close()

		f, err := os.Open(filename)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		io.Copy(stdin, f)
	}()

	out, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	return out
}
