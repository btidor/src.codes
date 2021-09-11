package publisher

import "net/url"

const Epoch = 2

type Distro struct {
	Name       string
	Mirror     *url.URL
	Areas      []string
	Components []string
}
