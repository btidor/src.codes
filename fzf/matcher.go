package fzf

import (
	"fmt"
	"unicode"
)

type Matcher struct {
	query  [][]qdat // by byte, then in reverse index order
	states []state
	target []byte
	epoch  int16
}

type qdat struct {
	index int
	boost uint
}

type state struct {
	score       uint
	consecutive int16
	epoch       int16
}

func (s state) Format(f fmt.State, c rune) {
	f.Write([]byte(fmt.Sprintf("% 3d:% 2d", s.score, s.consecutive)))
}

func NewMatcher(query string) Matcher {
	var qrunes = []rune(query)
	var lookup = make([][]qdat, 128)
	for i := len(qrunes) - 1; i >= 0; i-- {
		r := qrunes[i]
		lr := unicode.ToLower(r)
		if byte(r) == 0 || byte(r) > 127 || byte(lr) == 0 || byte(lr) > 127 {
			panic("query out of range")
		}
		lookup[r] = append(lookup[r], qdat{index: i, boost: 2})
		if r != lr {
			lookup[lr] = append(lookup[lr], qdat{index: i, boost: 1})
		}
	}
	return Matcher{
		query:  lookup,
		states: make([]state, len(query)),
	}
}

func (m *Matcher) Advance(component []byte) Matcher {
	st, ep, _ := m.advance(component)
	return Matcher{
		query:  m.query,
		states: st,
		target: append(m.target, []byte(component)...),
		epoch:  ep,
	}
}

func (m *Matcher) Score(component []byte) uint {
	_, _, sc := m.advance(component)
	return sc
}

func (m *Matcher) Target(component []byte) string {
	var target = append(m.target, component...)
	return string(target)
}

// Adapted from VS Code's fuzzy-find algorithm @
// https://github.com/microsoft/vscode/blob/55061e31/src/vs/base/common/fuzzyScorer.ts

func (m *Matcher) advance(component []byte) ([]state, int16, uint) {
	var states = m.states
	var scratch = make([]state, len(states))
	copy(scratch, states)

	var epoch = m.epoch
	var prev byte
	if len(m.target) > 0 {
		prev = m.target[len(m.target)-1]
	}

	var score uint

	for _, c := range component {
		var qdats = m.query[c]
		for _, qd := range qdats {
			var curr = scratch[qd.index]

			var next = state{
				score:       qd.boost,
				consecutive: 1,
				epoch:       epoch + 1,
			}

			var past state
			if qd.index != 0 {
				past = scratch[qd.index-1]
				if past.score == 0 {
					continue
				}
				next.score += past.score
				if past.epoch == epoch {
					// +5C points for C (prior) consecutive characters
					next.score += uint(past.consecutive) * 5
					next.consecutive = past.consecutive + 1
				}
			}

			// +8/+5/+4 points for start of path/component/word
			switch prev {
			case byte(0):
				next.score += 8
			case '/', '\\':
				next.score += 5
			case '_', '-', '.', ' ', '\'', '"', ':':
				next.score += 4
			default:
				// camel*C*ase bonus (if isUpper(c)), can't double-dip with
				// start-of-path/separator bonus
				if c >= 'A' && c <= 'Z' {
					next.score += 2
				}
			}

			// Handle scoring
			if next.score > curr.score {
				scratch[qd.index] = next
				if qd.index == len(scratch)-1 {
					score = next.score
				}
			}
		}
		// fmt.Printf("%c %#v\n", c, scratch)
		prev = c
		epoch++
	}

	return scratch, epoch, score
}
