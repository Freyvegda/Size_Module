package events

import (
	"sync"
	"testing"
	"time"
)

func TestHubDeliversToEverySubscriber(t *testing.T) {
	hub := NewHub()
	first, unsubscribeFirst := hub.Subscribe("job-1")
	defer unsubscribeFirst()
	second, unsubscribeSecond := hub.Subscribe("job-1")
	defer unsubscribeSecond()
	other, unsubscribeOther := hub.Subscribe("job-2")
	defer unsubscribeOther()

	hub.Publish("job-1", Event{Type: "progress", JobID: "job-1", Seq: 1})

	for name, ch := range map[string]<-chan Event{"first": first, "second": second} {
		select {
		case event := <-ch:
			if event.Type != "progress" || event.Seq != 1 {
				t.Fatalf("%s subscriber got %+v", name, event)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s subscriber received nothing", name)
		}
	}

	select {
	case event := <-other:
		t.Fatalf("job-2 subscriber should not receive job-1 events, got %+v", event)
	default:
	}

	if hub.SubscriberCount("job-1") != 2 {
		t.Fatalf("expected two subscribers, got %d", hub.SubscriberCount("job-1"))
	}
}

func TestHubUnsubscribeStopsDelivery(t *testing.T) {
	hub := NewHub()
	ch, unsubscribe := hub.Subscribe("job")
	unsubscribe()
	unsubscribe() // idempotent

	hub.Publish("job", Event{Type: "progress"})
	select {
	case event := <-ch:
		t.Fatalf("unsubscribed channel received %+v", event)
	case <-time.After(50 * time.Millisecond):
	}
	if hub.SubscriberCount("job") != 0 {
		t.Fatal("subscription should be gone")
	}
}

// A subscriber that never reads must not block the publisher.
func TestHubDropsEventsForSlowSubscribers(t *testing.T) {
	hub := NewHub()
	_, unsubscribe := hub.Subscribe("job")
	defer unsubscribe()

	done := make(chan struct{})
	go func() {
		for i := 0; i < subscriberBuffer*4; i++ {
			hub.Publish("job", Event{Type: "progress", Seq: int64(i)})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publishing blocked on a slow subscriber")
	}
}

func TestHubConcurrentPublishAndUnsubscribe(t *testing.T) {
	hub := NewHub()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				ch, unsubscribe := hub.Subscribe("job")
				_ = ch
				unsubscribe()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 500; j++ {
			hub.Publish("job", Event{Type: "progress"})
		}
	}()
	wg.Wait()
}
