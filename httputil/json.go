// Package httputil provides a shared HTTP client for the external data sources.
package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const userAgent = "Previsix (+https://github.com/romitou/previsix)"

var Client = &http.Client{Timeout: 30 * time.Second}

// GetJSON performs a GET request and decodes the JSON response into out.
func GetJSON(ctx context.Context, url string, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return do(request, out)
}

// PostJSON encodes body as JSON, posts it and decodes the JSON response into out.
func PostJSON(ctx context.Context, url string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return do(request, out)
}

func do(request *http.Request, out any) error {
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)

	response, err := Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("%s %s: unexpected status %s: %s", request.Method, request.URL.Redacted(), response.Status, bytes.TrimSpace(snippet))
	}

	if err = json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: decoding response: %w", request.Method, request.URL.Redacted(), err)
	}
	return nil
}
