package jsonx

import (
	"strings"
	"testing"
)

func TestViewFormatsJSON(t *testing.T) {
	got, err := View(strings.NewReader(`{"name":"Dex","items":[1,2]}`))
	if err != nil {
		t.Fatalf("View returned error: %v", err)
	}

	want := "{\n  \"items\": [\n    1,\n    2\n  ],\n  \"name\": \"Dex\"\n}\n"
	if got != want {
		t.Fatalf("formatted JSON mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestDiffReportsAddedRemovedAndChangedPaths(t *testing.T) {
	got, err := Diff(
		strings.NewReader(`{"name":"Dex","old":true,"nested":{"count":1}}`),
		strings.NewReader(`{"name":"Dex CLI","new":true,"nested":{"count":2}}`),
	)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}

	want := []Change{
		{Path: "name", Type: Changed, Before: "Dex", After: "Dex CLI"},
		{Path: "nested.count", Type: Changed, Before: float64(1), After: float64(2)},
		{Path: "new", Type: Added, After: true},
		{Path: "old", Type: Removed, Before: true},
	}
	if len(got) != len(want) {
		t.Fatalf("diff length mismatch\nwant: %#v\ngot: %#v", want, got)
	}
	for i := range want {
		if got[i].Path != want[i].Path || got[i].Type != want[i].Type || got[i].Before != want[i].Before || got[i].After != want[i].After {
			t.Fatalf("diff[%d] mismatch\nwant: %#v\ngot: %#v", i, want[i], got[i])
		}
	}
}

func TestQueryAndFlatten(t *testing.T) {
	value, err := Decode(strings.NewReader(`{"users":[{"name":"Ada","active":true}],"meta":{"count":1}}`))
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	got, err := Query(value, "users.0.name")
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if got != "Ada" {
		t.Fatalf("unexpected query value: %#v", got)
	}

	flat := Flatten(value)
	if flat["users.0.active"] != true || flat["meta.count"] != float64(1) {
		t.Fatalf("unexpected flatten result: %#v", flat)
	}
}

func TestMinifyValidateRedactAndFingerprint(t *testing.T) {
	minified, err := Minify(strings.NewReader("{\n  \"email\": \"a@example.com\",\n  \"token\": \"secret\"\n}\n"))
	if err != nil {
		t.Fatalf("Minify returned error: %v", err)
	}
	if minified != `{"email":"a@example.com","token":"secret"}` {
		t.Fatalf("unexpected minified JSON: %s", minified)
	}

	if err := Validate(strings.NewReader(minified)); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	value, err := Decode(strings.NewReader(minified))
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	redacted := Redact(value)
	flat := Flatten(redacted)
	if flat["email"] != "[redacted-email]" || flat["token"] != "[redacted-secret]" {
		t.Fatalf("unexpected redaction: %#v", flat)
	}

	fingerprint, err := Fingerprint(value)
	if err != nil {
		t.Fatalf("Fingerprint returned error: %v", err)
	}
	if fingerprint == "" {
		t.Fatal("expected fingerprint")
	}
}

func TestPathsKeysAndTypes(t *testing.T) {
	value, err := Decode(strings.NewReader(`{"name":"Dex","count":2,"items":[true]}`))
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	if got := Keys(value); len(got) != 3 || got[0] != "count" || got[1] != "items" || got[2] != "name" {
		t.Fatalf("unexpected keys: %#v", got)
	}
	if got := Paths(value); len(got) != 4 || got[0] != "count" || got[1] != "items" || got[2] != "items.0" || got[3] != "name" {
		t.Fatalf("unexpected paths: %#v", got)
	}
	types := Types(value)
	if types["items.0"] != "bool" || types["count"] != "number" {
		t.Fatalf("unexpected types: %#v", types)
	}
}
