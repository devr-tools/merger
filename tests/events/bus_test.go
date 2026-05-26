package events_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mergerhq/merger/internal/config"
	"github.com/mergerhq/merger/internal/events"
)

func TestMemoryBusPublishSubscribeAndClose(t *testing.T) {
	bus := events.NewMemoryBus()

	called := false
	err := bus.Subscribe(events.EventPROpened, func(_ context.Context, event events.Envelope) error {
		called = true
		if event.Type != events.EventPROpened {
			t.Fatalf("unexpected event type: %s", event.Type)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	err = bus.Publish(context.Background(), events.NewEnvelope(events.EventPROpened, "test", "payload"))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !called {
		t.Fatal("expected handler to be called")
	}

	if err := bus.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := bus.Publish(context.Background(), events.NewEnvelope(events.EventPROpened, "test", nil)); err == nil {
		t.Fatal("expected publish to fail after close")
	}
	if err := bus.Subscribe(events.EventPROpened, func(context.Context, events.Envelope) error { return nil }); err == nil {
		t.Fatal("expected subscribe to fail after close")
	}
}

func TestMemoryBusPropagatesHandlerError(t *testing.T) {
	bus := events.NewMemoryBus()
	expected := errors.New("boom")

	err := bus.Subscribe(events.EventRiskAssigned, func(context.Context, events.Envelope) error {
		return expected
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	err = bus.Publish(context.Background(), events.NewEnvelope(events.EventRiskAssigned, "test", nil))
	if !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestRecordingBusSavesPublishedEvents(t *testing.T) {
	recorder := &stubRecorder{}
	bus := events.NewRecordingBus(events.NewMemoryBus(), recorder)
	event := events.NewEnvelope(events.EventChangePacketCreated, "test", map[string]string{"id": "cp_1"})

	if err := bus.Publish(context.Background(), event); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if recorder.saved.ID != event.ID {
		t.Fatalf("expected recorder to save %q, got %q", event.ID, recorder.saved.ID)
	}
}

func TestNewBusFromConfig(t *testing.T) {
	bus, err := events.NewBusFromConfig(config.EventsConfig{Backend: "memory"})
	if err != nil {
		t.Fatalf("new bus: %v", err)
	}
	if err := bus.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, err = events.NewBusFromConfig(config.EventsConfig{Backend: "unsupported"})
	if err == nil {
		t.Fatal("expected unsupported backend to fail")
	}
}

type stubRecorder struct {
	saved events.Envelope
}

func (s *stubRecorder) SaveEvent(_ context.Context, event events.Envelope) error {
	s.saved = event
	return nil
}
