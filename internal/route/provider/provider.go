package provider

import (
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/route"
	provider "github.com/yusing/go-proxy/internal/route/provider/types"
	"github.com/yusing/go-proxy/internal/task"
	W "github.com/yusing/go-proxy/internal/watcher"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

type (
	Provider struct {
		ProviderImpl

		t      provider.Type
		routes route.Routes
	}
	ProviderImpl interface {
		fmt.Stringer
		ShortName() string
		IsExplicitOnly() bool
		loadRoutesImpl() (route.Routes, gperr.Error)
		NewWatcher() W.Watcher
		Logger() *zerolog.Logger
	}
)

const (
	providerEventFlushInterval = 300 * time.Millisecond
)

var ErrEmptyProviderName = errors.New("empty provider name")

func NewFileProvider(filename string) *Provider {
	return &Provider{
		t:            provider.TypeFile,
		ProviderImpl: FileProviderImpl(filename),
	}
}

func NewDockerProvider(name string, dockerHost string) *Provider {
	return &Provider{
		t:            provider.TypeDocker,
		ProviderImpl: DockerProviderImpl(name, dockerHost),
	}
}

func NewAgentProvider(cfg *agent.AgentConfig) *Provider {
	return &Provider{
		t: provider.TypeAgent,
		ProviderImpl: &AgentProvider{
			AgentConfig: cfg,
			docker:      DockerProviderImpl(cfg.Name(), cfg.FakeDockerHost()),
		},
	}
}

func (p *Provider) Type() provider.Type {
	return p.t
}

// to work with json marshaller.
func (p *Provider) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *Provider) startRoute(parent task.Parent, r *route.Route) gperr.Error {
	err := r.Start(parent)
	if err != nil {
		delete(p.routes, r.Alias)
		return err.Subject(r.Alias)
	}
	p.routes[r.Alias] = r
	return nil
}

// Start implements task.TaskStarter.
func (p *Provider) Start(parent task.Parent) gperr.Error {
	t := parent.Subtask("provider."+p.String(), false)

	errs := gperr.NewBuilder("routes error")
	for _, r := range p.routes {
		errs.Add(p.startRoute(t, r))
	}

	watcher := p.NewWatcher()
	eventQueue := events.NewEventQueue(
		t.Subtask("event_queue", false),
		providerEventFlushInterval,
		func(events []events.Event) {
			handler := p.newEventHandler()
			// routes' lifetime should follow the provider's lifetime
			handler.Handle(t, events)
			handler.Log()
		},
		func(err gperr.Error) {
			gperr.LogError("event error", err, p.Logger())
		},
	)
	eventQueue.Start(watcher.Events(t.Context()))

	if err := errs.Error(); err != nil {
		return err.Subject(p.String())
	}
	return nil
}

func (p *Provider) RangeRoutes(do func(string, *route.Route)) {
	for alias, r := range p.routes {
		do(alias, r)
	}
}

func (p *Provider) GetRoute(alias string) (r *route.Route, ok bool) {
	r, ok = p.routes[alias]
	return
}

func (p *Provider) loadRoutes() (routes route.Routes, err gperr.Error) {
	routes, err = p.loadRoutesImpl()
	if err != nil && len(routes) == 0 {
		return route.Routes{}, err
	}
	errs := gperr.NewBuilder()
	errs.Add(err)
	// check for exclusion
	// set alias and provider, then validate
	for alias, r := range routes {
		r.Alias = alias
		r.Provider = p.ShortName()
		if err := r.Validate(); err != nil {
			errs.Add(err.Subject(alias))
			delete(routes, alias)
			continue
		}
		if r.ShouldExclude() {
			delete(routes, alias)
			continue
		}
		r.FinalizeHomepageConfig()
	}
	return routes, errs.Error()
}

func (p *Provider) LoadRoutes() (err gperr.Error) {
	p.routes, err = p.loadRoutes()
	return
}

func (p *Provider) NumRoutes() int {
	return len(p.routes)
}
