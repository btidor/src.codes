// Package publisher contains common types used by the publisher.
package publisher

import "net/url"

// Epoch is the current version of the publisher. Bumping this number will cause
// every package's index files to be recomputed.
const Epoch = 4

// Distro represents an umbrella distribution like 'hirsute' or 'buster'.
type Distro struct {
	Name       string
	Mirror     *url.URL
	Areas      []string // 'security', 'updates', '', etc.
	Components []string // 'main', 'multiverse', etc.
}
