package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
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
	configfile   = filepath.Join(etcdir, "zerokube.conf")
	servicefiles = filepath.Join(etcdir, "services", "*.zero")
	lockfile     = filepath.Join(rundir, "zerokube.lock")
	statedirs    = filepath.Join(rundir, "zerokube.*")
)

var (
	forceStop    = time.Duration(-1)
	gracePeriod  = 10 * time.Second
	readTimeout  = 10 * time.Second
	writeTimeout = 60 * time.Second
)

type Zero struct {
	AdminHost string
	Docker    *client.Client
	// TODO: always reload from disk
	Configs map[string]Config
	// TODO: protect with mutex
	Services map[string][]Service
	Hooks    map[string]map[string]Config
	Token    string
}

type Meta struct {
	Token string `yaml:"token"`
}

type Config struct {
	Serve string            `yaml:"serve"`
	Image string            `yaml:"image"`
	Mount []mount.Mount     `yaml:"mount,omitempty"`
	Run   string            `yaml:"run,omitempty"`
	Hooks map[string]string `yaml:"hooks,omitempty"`
}

type Service struct {
	Slug      string
	Banner    string
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

	if _, err := os.Stat(configfile); err != nil {
		key := make([]byte, 33)
		_, err := rand.Read(key)
		if err != nil {
			panic(err)
		}
		str := base64.URLEncoding.EncodeToString(key)
		err = os.WriteFile(configfile,
			[]byte("token: zero_"+str+"\n"), 0600)
		if err != nil {
			panic(err)
		}
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

	var meta = Meta{}
	contents, err := os.ReadFile(configfile)
	if err != nil {
		panic(err)
	}
	if yaml.Unmarshal(contents, &meta); err != nil {
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
		Hooks:     make(map[string]map[string]Config),
		Token:     meta.Token,
	}
	var ctx = context.Background()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		for _, services := range zero.Services {
			for _, svc := range services {
				err := zero.Docker.ContainerStop(
					context.Background(), svc.Container, &gracePeriod,
				)
				if err == nil {
					fmt.Printf("Gracefully stopped %q\n", svc.Container)
				} else {
					fmt.Printf("Error stopping %q: %q\n", svc.Container, err)
				}
			}
		}
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
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
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
	for slug, config := range z.Configs {
		if config.Serve != "" {
			if _, ok := z.Services[config.Serve]; ok {
				err := fmt.Errorf("duplicated service: %s", config.Serve)
				panic(err)
			}
			z.StartService(ctx, slug)
		}
		for hook := range config.Hooks {
			if _, ok := z.Hooks[slug]; !ok {
				z.Hooks[slug] = make(map[string]Config)
			}
			if _, ok := z.Hooks[slug][hook]; ok {
				err := fmt.Errorf("duplicated hook: %s.%s", slug, hook)
				panic(err)
			}
			z.Hooks[slug][hook] = config
		}
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
		svc[len(svc)-1].Proxy.ServeHTTP(w, r)
	} else {
		internal.HTTPError(w, r, 502)
	}
}

func (z *Zero) ServeAdmin(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		fmt.Fprintf(w, "Hello from zerokube@%s!\n\nRunning Containers:\n", commit)
		containers, err := z.Docker.ContainerList(
			r.Context(), types.ContainerListOptions{})
		if err != nil {
			panic(err)
		}
		for _, container := range containers {
			fmt.Fprintf(w, "%s %s %s\n", container.ID[:10],
				container.Image, container.Names)
		}
		fmt.Fprint(w, "\nHooks:")
		for slug, configs := range z.Hooks {
			for hook := range configs {
				fmt.Fprintf(w, " %s.%s", slug, hook)
			}
		}
		fmt.Fprintln(w)
	case "/deploy":
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		slug := strings.TrimSpace(buf.String())
		if r.Method != "POST" {
			internal.HTTPError(w, r, 405)
		} else if config, ok := z.Configs[slug]; !ok {
			internal.HTTPError(w, r, 404)
		} else if subtle.ConstantTimeCompare([]byte("Bearer "+z.Token),
			[]byte(r.Header.Get("Authorization"))) != 1 {
			internal.HTTPError(w, r, 401)
		} else {
			name := z.StartService(r.Context(), slug)
			fmt.Fprintf(w, "Started %q\n", name)
			go func() {
				time.Sleep(readTimeout + writeTimeout)
				for _, svc := range z.Services[config.Serve] {
					if svc.Container == name {
						break
					}
					err := z.Docker.ContainerStop(
						context.Background(), svc.Container, &gracePeriod)
					if err == nil {
						fmt.Printf("Gracefully stopped %q\n", svc.Container)
					} else {
						fmt.Printf("Error stopping %q: %q\n", svc.Container, err)
					}
				}
			}()
		}
	case "/hook":
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		name := strings.SplitN(strings.TrimSpace(buf.String()), ".", 2)
		if r.Method != "POST" {
			internal.HTTPError(w, r, 405)
		} else if configs, ok := z.Hooks[name[0]]; !ok {
			internal.HTTPError(w, r, 404)
		} else if config, ok := configs[name[1]]; !ok {
			internal.HTTPError(w, r, 404)
		} else if subtle.ConstantTimeCompare([]byte("Bearer "+z.Token),
			[]byte(r.Header.Get("Authorization"))) != 1 {
			internal.HTTPError(w, r, 401)
		} else {
			z.RunHook(r.Context(), name[0], name[1], config)
			fmt.Fprintf(w, "Started \"%s.%s\"\n", name[0], name[1])
		}
	case "/robots.txt":
		if r.Method != "GET" {
			internal.HTTPError(w, r, 405)
		}
		fmt.Fprintf(w, "User-agent: *\nDisallow: /\n")
	default:
		internal.HTTPError(w, r, 404)
	}
}

func (z *Zero) StartService(ctx context.Context, slug string) string {
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

	// Wait for startup
	var banner string
	for i := time.Duration(0); i < gracePeriod; i += time.Second {
		time.Sleep(1 * time.Second)

		r := httptest.NewRequest("GET", config.Serve, nil)
		w := httptest.NewRecorder()
		proxy.ServeHTTP(w, r)

		buf := new(bytes.Buffer)
		buf.ReadFrom(w.Result().Body)
		banner = buf.String()

		if w.Result().StatusCode == 200 && banner != "" {
			fmt.Printf("Container %q started successfully\n", name)
			break
		} else {
			fmt.Printf("Waiting on %q: %q %q\n", name, w.Result().Status, banner)
		}
	}
	if banner == "" {
		err := z.Docker.ContainerStop(ctx, name, &forceStop)
		if err != nil {
			panic(err)
		}
		err = fmt.Errorf("container failed to start: %q", name)
		panic(err)
	}

	svc := Service{
		Slug:      slug,
		Banner:    banner,
		Config:    config,
		Container: name,
		Proxy:     proxy,
		StateDir:  stateDir,
	}
	z.Services[svc.Config.Serve] = append(z.Services[svc.Config.Serve], svc)

	return name
}

func (z *Zero) RunHook(ctx context.Context, slug string, hook string, config Config) {
	// Set up instance name
	id := make([]byte, 2)
	_, err := rand.Read(id)
	if err != nil {
		panic(err)
	}
	qualified := fmt.Sprintf("%s.%s", slug, hook)
	name := fmt.Sprintf("zerokube.%s.%s", qualified, hex.EncodeToString(id))

	// Parse and sanitize options
	cmd := make([]string, 0)
	raw, err := shlex.Split(config.Hooks[hook])
	if err != nil {
		panic(err)
	}
	for _, r := range raw {
		if r != "\n" {
			cmd = append(cmd, r)
		}
	}

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
			Mounts: config.Mount,
		}, nil, nil, name)
	if err != nil {
		panic(err)
	}

	err = z.Docker.ContainerStart(ctx, name, types.ContainerStartOptions{})
	if err != nil {
		panic(err)
	}
}
