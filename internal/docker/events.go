package docker

import (
	"context"
	"github.com/moby/moby/client"
	"log/slog"
	"time"
)

type EventsAPI interface {
	Events(context.Context, client.EventsListOptions) client.EventsResult
}
type EventNotice struct {
	Disconnected bool
	Reconnected  bool
}

// Watch reconnects with bounded exponential backoff. Each connection forces a
// complete reconciliation, so dropped events and replay gaps cannot freeze inventory.
func Watch(ctx context.Context, api EventsAPI) <-chan EventNotice {
	out := make(chan EventNotice, 16)
	go func() {
		defer close(out)
		backoff := time.Second
		send := func(v EventNotice) {
			select {
			case out <- v:
			case <-ctx.Done():
			}
		}
		for ctx.Err() == nil {
			streamCtx, cancel := context.WithCancel(ctx)
			stream := api.Events(streamCtx, client.EventsListOptions{Filters: client.Filters{}.Add("type", "container").Add("event", "start", "die", "destroy", "rename")})
			send(EventNotice{Reconnected: true})
			connectedAt := time.Now()
		streamLoop:
			for {
				select {
				case <-ctx.Done():
					cancel()
					return
				case _, ok := <-stream.Messages:
					if !ok {
						break streamLoop
					}
					send(EventNotice{})
				case err := <-stream.Err:
					slog.Warn("Docker events disconnected", "op", "events", "error", err)
					break streamLoop
				}
			}
			cancel()
			send(EventNotice{Disconnected: true})
			if time.Since(connectedAt) > 30*time.Second {
				backoff = time.Second
			}
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if backoff < 30*time.Second {
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
			}
		}
	}()
	return out
}
