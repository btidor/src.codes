package main

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
)

// Includes code borrowed from rsc's codesearch @
// github.com/google/codesearch@8ba29bd:regexp/match.go

type Grep struct {
	Context int
	Regexp  *regexp.Regexp
	Stdout  io.Writer

	buf []byte
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

func (g *Grep) Reader(r io.Reader, filename string) (int, error) {
	if g.buf == nil {
		g.buf = make([]byte, 1<<20)
	}
	var (
		buf    = g.buf[:0]
		lineno = 1
		count  = 0
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
		}
		chunkStart := 0
		for chunkStart < end {
			// Note: we require the `m` flag to be set so ^ and $ may always
			// match the start/end of a line. This is because we can't inform
			// the regexp engine where the start and end of the file are.
			pair := g.Regexp.FindIndex(buf[chunkStart:end])
			if pair == nil {
				break
			}
			matchStart := pair[0] + chunkStart
			matchEnd := pair[1] + chunkStart
			lineStart := bytes.LastIndex(buf[chunkStart:matchStart], nl) + 1 + chunkStart
			lineEnd := bytes.Index(buf[matchEnd:end], nl) + 1 + matchEnd
			if lineEnd < 0 {
				panic("could not find end of line")
			}
			lineno += countNL(buf[chunkStart:lineStart])
			count++
			fmt.Fprintf(g.Stdout, "%s:%d %q\n", filename, lineno, buf[lineStart:lineEnd])
			lineno++
			chunkStart = lineEnd
		}
		if err == nil {
			lineno += countNL(buf[chunkStart:end])
		}
		n = copy(buf, buf[end:]) // TODO: problem!
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
