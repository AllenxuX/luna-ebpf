package docker

import (
	"context"
	"errors"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"sync/atomic"
	"testing"
	"time"
)

type fakeEvents struct{ calls atomic.Int32 }

func (f *fakeEvents) Events(ctx context.Context, _ client.EventsListOptions) client.EventsResult {
	n := f.calls.Add(1)
	messages := make(chan events.Message, 1)
	errs := make(chan error, 1)
	if n == 1 {
		errs <- errors.New("disconnect")
	} else {
		messages <- events.Message{Action: "start"}
	}
	return client.EventsResult{Messages: messages, Err: errs}
}
func TestEventsReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api := &fakeEvents{}
	ch := Watch(ctx, api)
	deadline := time.After(4 * time.Second)
	reconnects := 0
	disconnected := false
	for {
		select {
		case n := <-ch:
			if n.Reconnected {
				reconnects++
			}
			if n.Disconnected {
				disconnected = true
			}
			if reconnects == 2 && disconnected {
				return
			}
		case <-deadline:
			t.Fatalf("calls=%d reconnects=%d disconnected=%v", api.calls.Load(), reconnects, disconnected)
		}
	}
}
