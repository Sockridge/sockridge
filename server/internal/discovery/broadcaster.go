package discovery

import (
	"sync"

	registryv1 "github.com/utsav-develops/SocialAgents/server/gen/go/agentregistry/v1"
)

// Broadcaster fans out WatchResponse events to all active Watch subscribers.
// Each call to Watch registers a channel; the registry service calls Publish
// whenever an agent is created, updated, or deprecated.
type Broadcaster struct {
	mu          sync.RWMutex
	subscribers map[string]chan *registryv1.WatchResponse // key = subscriber id
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[string]chan *registryv1.WatchResponse),
	}
}

func (b *Broadcaster) Subscribe(id string) chan *registryv1.WatchResponse {
	ch := make(chan *registryv1.WatchResponse, 32)
	b.mu.Lock()
	b.subscribers[id] = ch
	b.mu.Unlock()
	return ch
}

func (b *Broadcaster) Unsubscribe(id string) {
	b.mu.Lock()
	if ch, ok := b.subscribers[id]; ok {
		close(ch)
		delete(b.subscribers, id)
	}
	b.mu.Unlock()
}

func (b *Broadcaster) Publish(event *registryv1.WatchResponse) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// subscriber is slow — drop rather than block the publisher
		}
	}
}
