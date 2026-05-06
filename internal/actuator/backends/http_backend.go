package backends

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/loadequilibrium/loadequilibrium/internal/actuator"
)

type HTTPBackend struct {
	endpoint string
	client   *http.Client
}

func NewHTTPBackend(endpoint string) *HTTPBackend {
	return &HTTPBackend{
		endpoint: strings.TrimSpace(endpoint),
		client:   &http.Client{},
	}
}

func (b *HTTPBackend) Execute(ctx context.Context, snap actuator.DirectiveSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.endpoint == "" {
		return errors.New("http backend: endpoint not configured")
	}

	payload := struct {
		ServiceID   string  `json:"service_id"`
		ScaleFactor float64 `json:"scale_factor"`
	}{
		ServiceID:   snap.ServiceID,
		ScaleFactor: snap.ScaleFactor,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("http backend: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("http backend: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("http backend: execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		msg = bytes.TrimSpace(msg)
		if len(msg) > 0 {
			return fmt.Errorf("http backend: status=%d body=%s", resp.StatusCode, string(msg))
		}
		return fmt.Errorf("http backend: status=%d", resp.StatusCode)
	}

	log.Printf("[actuator:http] svc=%s scale=%.3f endpoint=%s status=%d tick=%d",
		snap.ServiceID, snap.ScaleFactor, b.endpoint, resp.StatusCode, snap.TickIndex)

	return nil
}
