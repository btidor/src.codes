package main

import (
	"bytes"
	"fmt"
	"io"

	"github.com/google/codesearch/regexp"
)

// Includes code borrowed from rsc's codesearch @
// github.com/google/codesearch@8ba29bd:regexp/match.go

type Grep struct {
	Regexp *regexp.Regexp
	Stdout io.Writer

	match bool
	buf   []byte
}

var nl = []byte{'\n'}

func countNL(b []byte) int {
	n := 0
	for {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			break
		}
		n++
		b = b[i+1:]
	}
	return n
}

func (g *Grep) Reader(r io.Reader, name string) (int, error) {
	if g.buf == nil {
		g.buf = make([]byte, 1<<20)
	}
	var (
		buf       = g.buf[:0]
		lineno    = 1
		count     = 0
		prefix    = name + ":"
		beginText = true
		endText   = false
	)
	for {
		n, err := io.ReadFull(r, buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		end := len(buf)
		if err == nil {
			i := bytes.LastIndex(buf, nl)
			if i >= 0 {
				end = i + 1
			}
		} else {
			endText = true
		}
		chunkStart := 0
		for chunkStart < end {
			m1 := g.Regexp.Match(buf[chunkStart:end], beginText, endText) + chunkStart
			beginText = false
			if m1 < chunkStart {
				break
			}
			g.match = true
			lineStart := bytes.LastIndex(buf[chunkStart:m1], nl) + 1 + chunkStart
			lineEnd := m1 + 1
			if lineEnd > end {
				lineEnd = end
			}
			lineno += countNL(buf[chunkStart:lineStart])
			line := buf[lineStart:lineEnd]
			nl := ""
			if len(line) == 0 || line[len(line)-1] != '\n' {
				nl = "\n"
			}
			count++
			fmt.Fprintf(g.Stdout, "%s%d:%s%s", prefix, lineno, line, nl)
			lineno++
			chunkStart = lineEnd
		}
		if err == nil {
			lineno += countNL(buf[chunkStart:end])
		}
		n = copy(buf, buf[end:])
		buf = buf[:n]
		if len(buf) == 0 && err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				return count, err
			}
			break
		}
	}
	return count, nil
}
