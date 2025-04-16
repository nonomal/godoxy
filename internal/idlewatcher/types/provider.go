package idlewatcher

import (
	"context"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

type Provider interface {
	ContainerPause(ctx context.Context) error
	ContainerUnpause(ctx context.Context) error
	ContainerStart(ctx context.Context) error
	ContainerStop(ctx context.Context, signal Signal, timeout int) error
	ContainerKill(ctx context.Context, signal Signal) error
	ContainerStatus(ctx context.Context) (ContainerStatus, error)
	Watch(ctx context.Context) (eventCh <-chan events.Event, errCh <-chan gperr.Error)
	Close()
}
