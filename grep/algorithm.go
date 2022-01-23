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
		// Read into end of buffer, up to capacity. buf[:end] is the range of
		// data that will be searched this iteration of the loop.
		n, err := io.ReadFull(r, buf[len(buf):cap(buf)])
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return count, err
		}
		buf = buf[:len(buf)+n]
		end := len(buf)
		if err == nil {
			// Not yet at EOF, so limit buffer to last complete line
			i := bytes.LastIndex(buf, nl)
			if i >= 0 {
				end = i + 1
			}
		}

		// Search the buffer for non-overlapping matches. After each iteration,
		// chunkStart is the index just past the end of the latest match.
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
				// Can't find end of line: this can happen when we match the
				// last line of a file that doesn't have a trailing newline.
				lineEnd = end
			}
			lineno += countNL(buf[chunkStart:lineStart])
			fmt.Fprintf(g.Stdout, "%s:%d %q\n", filename, lineno, buf[lineStart:lineEnd])
			lineno += countNL(buf[lineStart:lineEnd])
			chunkStart = lineEnd
			count++
		}

		// What if there's a valid match here, but it runs past the end of the
		// buffer? We slide up the second half of the buffer and continue the
		// search, so we'll find it on the next iteration. (Matches longer than
		// 50% of the buffer capacity are not guaranteed to be found.) After
		// this, buf[:len(buf)] is the unmatched remainder.
		midpoint := cap(buf) / 2
		if midpoint > end {
			midpoint = end
		}
		pivot := bytes.Index(buf[midpoint:end], nl) + 1 + midpoint
		if chunkStart < pivot {
			lineno += countNL(buf[chunkStart:pivot])
			chunkStart = pivot
		}
		lineno += countNL(buf[chunkStart:end])
		n = copy(buf, buf[chunkStart:])
		buf = buf[:n]
		if err != nil {
			break // EOF
		}
	}
	return count, nil
}
