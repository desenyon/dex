package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type Response struct {
	Method     string              `json:"method"`
	URL        string              `json:"url"`
	Status     string              `json:"status"`
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body,omitempty"`
	Duration   string              `json:"duration"`
}

type Session struct {
	Name      string     `json:"name"`
	CreatedAt string     `json:"created_at,omitempty"`
	Responses []Response `json:"responses"`
}

func Request(ctx context.Context, method string, target string, bodyPath string) (Response, error) {
	var body io.Reader
	if bodyPath != "" {
		data, err := os.ReadFile(bodyPath)
		if err != nil {
			return Response{}, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return Response{}, err
	}
	if bodyPath != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return Response{}, err
	}
	return Response{
		Method:     method,
		URL:        target,
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       string(data),
		Duration:   time.Since(start).Round(time.Millisecond).String(),
	}, nil
}

func InferSchema(reader io.Reader) (map[string]string, error) {
	var value any
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema inference expects a JSON object")
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(object))
	for _, key := range keys {
		out[key] = typeName(object[key])
	}
	return out, nil
}

func SaveSession(path string, session Session) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func LoadSession(path string) (Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func typeName(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "bool"
	case json.Number:
		return "number"
	case nil:
		return "null"
	default:
		return strings.TrimPrefix(fmt.Sprintf("%T", value), "json.")
	}
}
