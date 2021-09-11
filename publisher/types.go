package publisher

import "net/url"

const Epoch = 0

type Distro struct {
	Name       string
	Mirror     *url.URL
	Areas      []string
	Components []string
}
