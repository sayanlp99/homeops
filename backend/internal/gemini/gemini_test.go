package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fake(t *testing.T, answer func(model string, w http.ResponseWriter)) *Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "k" {
			t.Errorf("key not sent")
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Config["responseJsonSchema"] == nil {
			t.Errorf("bad request: %v", err)
		}
		model := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ":generateContent")
		answer(model, w)
	}))
	t.Cleanup(srv.Close)
	c := New("k", "first", "second")
	c.base = srv.URL + "/"
	return c
}

func TestJSONDecodesAnswer(t *testing.T) {
	c := fake(t, func(_ string, w http.ResponseWriter) {
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"name\":\"Potato\"}"}]}}]}`))
	})
	var out struct{ Name string }
	if err := c.JSON(context.Background(), "sys", "aloo", map[string]any{"type": "object"}, &out); err != nil || out.Name != "Potato" {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestJSONFallsBackToNextModel(t *testing.T) {
	var asked []string
	c := fake(t, func(model string, w http.ResponseWriter) {
		asked = append(asked, model)
		if model == "first" {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":{"code":503,"message":"overloaded","status":"UNAVAILABLE"}}`))
			return
		}
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{}"}]}}]}`))
	})
	var out map[string]any
	if err := c.JSON(context.Background(), "", "x", map[string]any{}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Join(asked, ",") != "first,second" {
		t.Fatalf("asked %v", asked)
	}
}

func TestJSONQuotaStops(t *testing.T) {
	calls := 0
	c := fake(t, func(_ string, w http.ResponseWriter) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}`))
	})
	var out map[string]any
	if err := c.JSON(context.Background(), "", "x", map[string]any{}, &out); !errors.Is(err, ErrQuota) || calls != 1 {
		t.Fatalf("err %v after %d calls", err, calls)
	}
}

func TestNewWithoutKeyIsOff(t *testing.T) {
	if New("", "m") != nil {
		t.Fatal("expected nil client")
	}
}
