package apt

import (
	"log"
	"net/url"
	"path"

	"github.com/btidor/src.codes/internal"
	"github.com/btidor/src.codes/publisher"
	"github.com/btidor/src.codes/publisher/control"
)

type Source struct {
	Distro    string
	Area      string
	Component string

	SourceIndex  *url.URL
	DownloadBase *url.URL
}

func (s Source) Slug() string {
	x := s.Distro
	if s.Area != "" {
		x += "-" + s.Area
	}
	return x + ":" + s.Component
}

func FetchSources(distro publisher.Distro) []Source {
	var sources []Source
	for _, area := range distro.Areas {
		slug := distro.Name
		if area != "" {
			slug += "-" + area
		}

		release := internal.DownloadFile(distro.Mirror, "dists", slug, "Release")
		dsc, err := control.Parse(release.String())
		if err != nil {
			panic(err)
		}

		files := dsc.GetFiles("SHA256")
		for _, component := range distro.Components {
			file := control.FindFileInList(files, path.Join(component, "source", "Sources.xz"))
			if file.Size == 0 {
				log.Printf("[%s:%s] WARNING: source index has length zero, skipping", slug, component)
				break
			}
			url := internal.URLWithPath(distro.Mirror, "dists", slug, component, "source",
				"by-hash", "SHA256", file.Hash)
			sources = append(sources, Source{
				Distro:       distro.Name,
				Area:         area,
				Component:    component,
				SourceIndex:  url,
				DownloadBase: distro.Mirror,
			})
		}
	}
	return sources
}
