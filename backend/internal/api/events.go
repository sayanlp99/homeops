package api

import (
	"fmt"
	"net/http"
	"time"

	"homeops/internal/realtime"
)

type topic string

const (
	eventTasks         topic = "tasks"
	eventSprints       topic = "sprints"
	eventRecurring     topic = "recurring"
	eventLabels        topic = "labels"
	eventMembers       topic = "members"
	eventHousehold     topic = "household"
	eventNotifications topic = "notifications"
)

// from builds an event attributed to the request's client (nil for background jobs).
func (t topic) from(r *http.Request) realtime.Event {
	ev := realtime.Event{Topic: string(t)}
	if r != nil {
		ev.Origin = clientID(r)
	}
	return ev
}

// events streams change notifications over Server-Sent Events. Comments every 20s
// keep Cloudflare's tunnel from closing an idle connection.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Time{})

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch, unsubscribe := s.hub.Subscribe(u.HouseholdID, u.ID)
	defer unsubscribe()

	fmt.Fprint(w, "retry: 3000\nevent: ready\ndata: {}\n\n")
	rc.Flush()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.stopping:
			return
		case msg := <-ch:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
				return
			}
			rc.Flush()
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			rc.Flush()
		}
	}
}

type realtimeEvent = realtime.Event
