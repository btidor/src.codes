package main

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
)

// Includes code adapted from rsc's codesearch @
// github.com/google/codesearch@8ba29bd:regexp/match.go

type Grep struct {
	Context int
	Regexp  *regexp.Regexp
	Stdout  io.Writer

	buf []byte
}

func (g *Grep) Reader(r io.Reader, filename string) (int, error) {
	if g.buf == nil {
		g.buf = make([]byte, maxFileSize)
	}
	var (
		buf        = g.buf[:0]
		chunkStart = 0 // index of end of last match in buf
		lineno     = 1 // 1-indexed line number of the line starting at chunkStart
		count      = 0
	)

	// Read the full file into buffer.
	n, err := io.ReadFull(r, buf[:cap(buf)])
	if err == nil {
		return 0, fmt.Errorf("cannot search file %q, larger than 1M", filename)
	} else if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return 0, err
	}
	buf = buf[:n]

	// Search the buffer for non-overlapping matches. After each iteration,
	// chunkStart is the index just past the end of the latest match.
	for chunkStart < len(buf) {
		// Note: we require the `m` flag to be set so ^ and $ may always
		// match the start/end of a line. This is because we can't inform
		// the regexp engine where the start and end of the file are.
		pair := g.Regexp.FindIndex(buf[chunkStart:])
		if pair == nil {
			break
		}
		matchStart := pair[0] + chunkStart
		matchEnd := pair[1] + chunkStart

		contextStart := bytes.LastIndexByte(buf[:matchStart], nl) + 1
		for i := 0; i < g.Context; i++ {
			contextStart = bytes.LastIndexByte(buf[:contextStart-1], nl) + 1
		}
		lineEnd := bytes.IndexByte(buf[matchEnd-1:], nl)
		if lineEnd < 0 {
			// Can't find end of line: this can happen when we match the
			// last line of a file that doesn't have a trailing newline.
			lineEnd = len(buf)
		} else {
			lineEnd += matchEnd
		}
		contextEnd := lineEnd
		for i := 0; i < g.Context; i++ {
			pos := bytes.IndexByte(buf[contextEnd:], nl)
			if pos < 0 {
				contextEnd = len(buf)
			} else {
				contextEnd = pos + 1 + contextEnd
			}
		}

		// Print result, then advance to the end of the last line containing the
		// match. This imposes a one-match-per-line resource limit, and ensures
		// buf[chunkStart:] is always on a line boundary so ^ works correctly.
		lineno += bytes.Count(buf[chunkStart:contextStart], []byte{nl})
		fmt.Fprintf(g.Stdout, "%s:%d %q\n", filename, lineno, buf[contextStart:contextEnd])
		lineno += bytes.Count(buf[contextStart:lineEnd], []byte{nl})
		chunkStart = lineEnd
		count++
	}

	return count, nil
}
