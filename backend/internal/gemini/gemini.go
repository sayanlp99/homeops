// Package gemini calls Google's Gemini API for small structured jobs: the prompt goes
// in, JSON matching a schema comes out. Nothing here knows about HomeOps data.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrQuota means Google refused for rate or daily limits (free tier: quite low).
var ErrQuota = errors.New("gemini: quota exceeded")

type Client struct {
	key    string
	models []string // tried in order; a model that times out or is overloaded falls through
	http   *http.Client
	base   string
}

// New returns nil when key is empty, so callers can treat AI as switched off.
func New(key string, models ...string) *Client {
	if key == "" {
		return nil
	}
	return &Client{key: key, models: models, http: &http.Client{}, base: "https://generativelanguage.googleapis.com/v1beta/models/"}
}

type part struct {
	Text string `json:"text"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type request struct {
	System   *content       `json:"systemInstruction,omitempty"`
	Contents []content      `json:"contents"`
	Config   map[string]any `json:"generationConfig"`
}

type response struct {
	Candidates []struct {
		Content      content `json:"content"`
		FinishReason string  `json:"finishReason"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// JSON asks for an answer shaped by schema (a JSON Schema object) and decodes it into out.
func (c *Client) JSON(ctx context.Context, system, prompt string, schema map[string]any, out any) error {
	body, _ := json.Marshal(request{
		System:   &content{Parts: []part{{system}}},
		Contents: []content{{Role: "user", Parts: []part{{prompt}}}},
		Config:   map[string]any{"responseMimeType": "application/json", "responseJsonSchema": schema, "temperature": 0.4},
	})
	var err error
	for _, m := range c.models {
		var text string
		if text, err = c.generate(ctx, m, body); err == nil {
			if err = json.Unmarshal([]byte(text), out); err != nil {
				return fmt.Errorf("gemini: unreadable answer: %w", err)
			}
			return nil
		}
		if errors.Is(err, ErrQuota) || ctx.Err() != nil {
			return err
		}
	}
	return err
}

func (c *Client) generate(ctx context.Context, model string, body []byte) (string, error) {
	// Each model gets its own share of the time, so a stuck one leaves room for the next.
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+model+":generateContent", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.key)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini %s: %w", model, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("gemini %s: HTTP %d", model, resp.StatusCode)
	}
	if r.Error != nil {
		if r.Error.Code == http.StatusTooManyRequests || r.Error.Status == "RESOURCE_EXHAUSTED" {
			return "", ErrQuota
		}
		return "", fmt.Errorf("gemini %s: %d %s", model, r.Error.Code, r.Error.Message)
	}
	if len(r.Candidates) == 0 {
		return "", fmt.Errorf("gemini %s: no answer", model)
	}
	var sb strings.Builder
	for _, p := range r.Candidates[0].Content.Parts {
		sb.WriteString(p.Text)
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("gemini %s: empty answer (%s)", model, r.Candidates[0].FinishReason)
	}
	return sb.String(), nil
}
