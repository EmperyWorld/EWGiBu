package httpapi

import (
	"testing"
	"time"
)

func TestHubOverflowEmitsResync(t *testing.T) {
	h := newEventHub(3)
	c := h.subscribe("")
	defer h.unsubscribe(c)
	// drain backlog
	time.Sleep(20 * time.Millisecond)
	for len(c.ch) > 0 {
		<-c.ch
	}
	h.publish(ManifestEvent{Kind: EventChanged, Revision: "a"})
	h.publish(ManifestEvent{Kind: EventChanged, Revision: "b"})
	h.publish(ManifestEvent{Kind: EventChanged, Revision: "c"})
	h.publish(ManifestEvent{Kind: EventChanged, Revision: "d"}) // overflow

	deadline := time.After(2 * time.Second)
	sawResync := false
	for !sawResync {
		select {
		case ev := <-c.ch:
			if ev.Kind == EventResyncRequired {
				sawResync = true
			}
		case <-deadline:
			t.Fatal("expected resyncRequired on overflow")
		}
	}
}

func TestReplayMissedIDForcesResync(t *testing.T) {
	h := newEventHub(10)
	h.publish(ManifestEvent{Kind: EventChanged, Revision: "x"})
	c := h.subscribe("unknown-id")
	defer h.unsubscribe(c)
	select {
	case ev := <-c.ch:
		if ev.Kind != EventResyncRequired {
			t.Fatalf("got %v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}
