package api

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInferSchema(t *testing.T) {
	schema, err := InferSchema(strings.NewReader(`{"id":1,"name":"Dex","tags":["cli"],"ok":true}`))
	if err != nil {
		t.Fatalf("InferSchema returned error: %v", err)
	}
	if schema["id"] != "number" || schema["name"] != "string" || schema["tags"] != "array" || schema["ok"] != "bool" {
		t.Fatalf("unexpected schema: %#v", schema)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.dexapi")
	session := Session{
		Name: "smoke",
		Responses: []Response{{
			Method:     "GET",
			URL:        "https://example.com",
			Status:     "200 OK",
			StatusCode: 200,
			Body:       "ok",
		}},
	}

	if err := SaveSession(path, session); err != nil {
		t.Fatalf("SaveSession returned error: %v", err)
	}
	got, err := LoadSession(path)
	if err != nil {
		t.Fatalf("LoadSession returned error: %v", err)
	}
	if got.Name != "smoke" || len(got.Responses) != 1 || got.Responses[0].StatusCode != 200 {
		t.Fatalf("unexpected session: %#v", got)
	}
}
