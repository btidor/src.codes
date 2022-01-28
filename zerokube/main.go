package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
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
	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/yaml.v2"
)

var (
	commit = "dev"
	etcdir = "/etc/zerokube"
	rundir = "/var/run/zerokube"

	acmedir      = filepath.Join(etcdir, "acme")
	servicefiles = filepath.Join(etcdir, "services", "*.zero")
	lockfile     = filepath.Join(rundir, "zerokube.lock")
	statedirs    = filepath.Join(rundir, "zerokube.*")
)

var forceStop = time.Duration(-1)

type Zero struct {
	AdminHost string
	Docker    *client.Client
	Configs   map[string]Config
	Services  map[string][]Service
}

type Config struct {
	Serve string        `yaml:"serve"`
	Image string        `yaml:"image"`
	Mount []mount.Mount `yaml:"mount,omitempty"`
	Run   string        `yaml:"run,omitempty"`
}

type Service struct {
	Slug      string
	Config    Config
	Container string
	Proxy     *httputil.ReverseProxy
	StateDir  string
}

func init() {
	if len(commit) > 7 {
		commit = commit[:7]
	}

	if err := os.MkdirAll(rundir, 0755); err != nil {
		panic(err)
	}
	lock := fslock.New(lockfile)
	if err := lock.LockWithTimeout(3 * time.Second); err != nil {
		fmt.Printf("Another zerokube process exists, exiting...\n")
		os.Exit(1)
	}
}

func main() {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	docker, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	var configs = make(map[string]Config)
	configFiles, err := filepath.Glob(servicefiles)
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

	var zero = &Zero{
		AdminHost: hostname,
		Docker:    docker,
		Configs:   configs,
		Services:  make(map[string][]Service),
	}
	var ctx = context.Background()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		fmt.Println()
		// zero.GracefulShutdown(ctx) // TODO
		os.Exit(130)
	}()

	zero.Initialize(ctx)

	var acme = autocert.Manager{
		Prompt: func(_ string) bool { return true },
		Cache:  autocert.DirCache(acmedir),
		HostPolicy: func(_ context.Context, host string) error {
			if host == zero.AdminHost {
				return nil
			} else if _, ok := zero.Services["https://"+host]; ok {
				return nil
			}
			return fmt.Errorf("refusing to issue certificate for unknown host: %s", host)
		},
	}
	var svr1 = http.Server{
		Handler: zero,
	}
	var svr2 = http.Server{
		Handler:      zero,
		TLSConfig:    acme.TLSConfig(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	fmt.Println("Listening on :80 and :443")
	go func() {
		err := svr1.ListenAndServe()
		panic(err)
	}()
	go func() {
		err := svr2.ListenAndServeTLS("", "")
		panic(err)
	}()
	select {}
}

func (z *Zero) Initialize(ctx context.Context) {
	// Clean up leftover Docker containers
	containers, err := z.Docker.ContainerList(
		ctx, types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}
	for _, container := range containers {
		if len(container.Names) > 0 &&
			strings.HasPrefix(container.Names[0], "/zerokube.") {
			fmt.Printf("Cleaning up %q\n", container.Names[0][1:])
			err := z.Docker.ContainerStop(ctx, container.ID, &forceStop)
			if err != nil {
				panic(err)
			}
		}
	}

	// Clean up leftover state directories
	dirs, err := filepath.Glob(statedirs)
	if err != nil {
		panic(err)
	}
	for _, dir := range dirs {
		err = os.RemoveAll(dir)
		if err != nil {
			panic(err)
		}
	}

	// Start requested containers
	for slug := range z.Configs {
		svc := z.StartContainer(ctx, slug)
		z.Services[svc.Config.Serve] = append(z.Services[svc.Config.Serve], svc)
	}
}

func (z *Zero) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Upgrade insecure requests
	if r.TLS == nil {
		var url = *r.URL
		url.Scheme = "https"
		url.Host = r.Host
		w.Header().Add("Location", url.String())
		w.WriteHeader(302)
		return
	}

	// Figure out which backend to route the request to
	var hostname = r.TLS.ServerName
	if hostname == z.AdminHost {
		z.ServeAdmin(w, r)
	} else if svc, ok := z.Services["https://"+hostname]; ok {
		svc[0].Proxy.ServeHTTP(w, r)
	} else {
		internal.HTTPError(w, r, 502)
	}

}

func (z *Zero) ServeAdmin(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		fmt.Fprintf(w, "Hello from zerokube@%s!\n", commit)
		containers, err := z.Docker.ContainerList(
			r.Context(), types.ContainerListOptions{})
		if err != nil {
			panic(err)
		}
		for _, container := range containers {
			fmt.Fprintf(w, "%s %s %s\n", container.ID[:10],
				container.Image, container.Names)
		}
	case "/robots.txt":
		fmt.Fprintf(w, "User-agent: *\nDisallow: /\n")
	default:
		internal.HTTPError(w, r, 404)
	}
}

func (z *Zero) StartContainer(ctx context.Context, slug string) Service {
	config, ok := z.Configs[slug]
	if !ok {
		err := fmt.Errorf("Config %q not found", slug)
		panic(err)
	}

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

	// Pull latest image
	reader, err := z.Docker.ImagePull(
		ctx, config.Image, types.ImagePullOptions{})
	if err != nil {
		panic(err)
	}
	io.Copy(os.Stdout, reader)
	reader.Close()

	// Run container
	_, err = z.Docker.ContainerCreate(ctx,
		&container.Config{
			Image: config.Image,
			Cmd:   cmd,
		}, &container.HostConfig{
			Mounts: mounts,
		}, nil, nil, name)
	if err != nil {
		panic(err)
	}

	err = z.Docker.ContainerStart(ctx, name, types.ContainerStartOptions{})
	if err != nil {
		panic(err)
	}

	// Set up proxy
	var proxy = &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL.Scheme = "http"
			r.URL.Host = r.Host
			r.Header.Del("X-Forwarded-For")
		},
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", filepath.Join(stateDir, "http.sock"))
			},
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		FlushInterval: -1,
	}

	return Service{
		Slug:      slug,
		Config:    config,
		Container: name,
		Proxy:     proxy,
		StateDir:  stateDir,
	}
}
