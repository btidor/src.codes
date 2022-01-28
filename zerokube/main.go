package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/btidor/src.codes/internal"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/google/shlex"
	"github.com/juju/fslock"
	"gopkg.in/yaml.v2"
)

var commit string = "dev"

var etcdir string = "/etc/zerokube"
var rundir string = "/var/run/zerokube"

var forceStop = time.Duration(-1)

var cli *client.Client
var configs = make(map[string]Config)
var instances = make(map[string][]Instance)

type Config struct {
	Serve string        `yaml:"serve"`
	Image string        `yaml:"image"`
	Mount []mount.Mount `yaml:"mount,omitempty"`
	Run   string        `yaml:"run,omitempty"`
}

type Instance struct {
	Name     string
	StateDir string
}

func main() {
	ctx := context.Background()

	var err error
	cli, err = client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	cleanup(ctx)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		fmt.Println()
		cleanup(ctx)
		os.Exit(130)
	}()

	configFiles, err := filepath.Glob(
		filepath.Join(etcdir, "services", "*.zero"),
	)
	if err != nil {
		panic(err)
	}
	for _, configFile := range configFiles {
		contents, err := os.ReadFile(configFile)
		if err != nil {
			panic(err)
		}
		config := Config{}
		if err := yaml.Unmarshal(contents, &config); err != nil {
			panic(err)
		}
		slug := strings.TrimSuffix(filepath.Base(configFile), ".zero")
		configs[slug] = config
	}

	for slug, config := range configs {
		instances[slug] = []Instance{
			startContainer(ctx, slug, config),
		}
	}

	server := &http.Server{
		Handler:      zero{},
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	fmt.Println("Listening on :4040")
	server.Addr = ":4040"
	err = server.ListenAndServe()
	panic(err)
}

type zero struct{}

func (z zero) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		fmt.Fprintf(w, "Hello from zerokube@%s!\n", commit)
		containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{})
		if err != nil {
			panic(err)
		}
		for _, container := range containers {
			fmt.Fprintf(w, "%s %s %s\n", container.ID[:10], container.Image, container.Names)
		}
	case "/robots.txt":
		fmt.Fprintf(w, "User-agent: *\nDisallow: /\n")
	default:
		internal.HTTPError(w, r, 404)
	}
}

func init() {
	if len(commit) > 7 {
		commit = commit[:7]
	}

	if err := os.MkdirAll(rundir, 0755); err != nil {
		panic(err)
	}
	lock := fslock.New(filepath.Join(rundir, "zerokube.lock"))
	if err := lock.LockWithTimeout(3 * time.Second); err != nil {
		fmt.Printf("Another zerokube process exists, exiting...\n")
		os.Exit(1)
	}
}

func cleanup(ctx context.Context) {
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}
	for _, container := range containers {
		if len(container.Names) > 0 && strings.HasPrefix(container.Names[0], "/zerokube.") {
			fmt.Printf("Cleaning up %q\n", container.Names[0])
			err := cli.ContainerStop(ctx, container.ID, &forceStop)
			if err != nil {
				panic(err)
			}
		}
	}

	statedirs, err := filepath.Glob(filepath.Join(rundir, "zerokube.*"))
	if err != nil {
		panic(err)
	}
	for _, dir := range statedirs {
		err = os.RemoveAll(dir)
		if err != nil {
			panic(err)
		}
	}
}

func startContainer(ctx context.Context, slug string, config Config) Instance {
	// Set up instance name and state directory
	id := make([]byte, 2)
	_, err := rand.Read(id)
	if err != nil {
		panic(err)
	}
	name := fmt.Sprintf("zerokube.%s.%s", slug, hex.EncodeToString(id))

	stateDir := filepath.Join(rundir, name)
	err = os.Mkdir(stateDir, 0755)
	if err != nil {
		panic(err)
	}

	// Parse and sanitize options
	cmd := make([]string, 0)
	raw, err := shlex.Split(config.Run)
	if err != nil {
		panic(err)
	}
	for _, r := range raw {
		if r != "\n" {
			cmd = append(cmd, r)
		}
	}

	mounts := append(config.Mount, mount.Mount{
		Type:   mount.TypeBind,
		Source: stateDir,
		Target: "/var/run/hyper",
	})

	// docker pull && docker run
	img, err := cli.ImagePull(ctx, config.Image, types.ImagePullOptions{})
	if err != nil {
		panic(err)
	}
	img.Close()

	_, err = cli.ContainerCreate(ctx, &container.Config{
		Image: config.Image,
		Cmd:   cmd,
	}, &container.HostConfig{
		Mounts: mounts,
	}, nil, nil, name)
	if err != nil {
		panic(err)
	}

	err = cli.ContainerStart(context.Background(), name, types.ContainerStartOptions{})
	if err != nil {
		panic(err)
	}

	return Instance{
		Name:     name,
		StateDir: stateDir,
	}
}
