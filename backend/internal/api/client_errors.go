package api

import (
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Crash reports from the app. A phone that shows a blank screen can't show its error,
// so the app sends it here and it lands in the server log.

type clientError struct {
	Message string `json:"message"`
	Stack   string `json:"stack"`
	URL     string `json:"url"`
	Build   string `json:"build"`
}

// At most a few reports a minute, so a crash loop can't flood the log.
var clientErrorLimit = struct {
	sync.Mutex
	window time.Time
	n      int
}{}

func (s *Server) reportClientError(w http.ResponseWriter, r *http.Request) error {
	var e clientError
	if err := decode(r, &e); err != nil {
		return err
	}
	clientErrorLimit.Lock()
	if time.Since(clientErrorLimit.window) > time.Minute {
		clientErrorLimit.window, clientErrorLimit.n = time.Now(), 0
	}
	clientErrorLimit.n++
	over := clientErrorLimit.n > 10
	clientErrorLimit.Unlock()
	if over {
		return noContent(w)
	}
	user := ""
	if u := currentUser(r); u != nil {
		user = u.Username
	}
	slog.Warn("app error", "user", user, "message", clip(e.Message, 500), "url", clip(e.URL, 300), "build", clip(e.Build, 40),
		"agent", clip(r.UserAgent(), 200), "stack", clip(e.Stack, 2000))
	return noContent(w)
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
