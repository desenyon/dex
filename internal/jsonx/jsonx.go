package jsonx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type ChangeType string

const (
	Added   ChangeType = "added"
	Removed ChangeType = "removed"
	Changed ChangeType = "changed"
)

type Change struct {
	Path   string     `json:"path"`
	Type   ChangeType `json:"type"`
	Before any        `json:"before,omitempty"`
	After  any        `json:"after,omitempty"`
}

func Decode(reader io.Reader) (any, error) {
	var value any
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return normalizeNumbers(value), nil
}

func View(reader io.Reader) (string, error) {
	value, err := Decode(reader)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func Minify(reader io.Reader) (string, error) {
	value, err := Decode(reader)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func Validate(reader io.Reader) error {
	var value any
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	return decoder.Decode(&value)
}

func Query(value any, path string) (any, error) {
	current := value
	if path == "" {
		return current, nil
	}
	for _, part := range strings.Split(path, ".") {
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[part]
			if !ok {
				return nil, fmt.Errorf("path %q not found", path)
			}
			current = next
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, fmt.Errorf("array index %q not found in path %q", part, path)
			}
			current = typed[index]
		default:
			return nil, fmt.Errorf("path %q cannot continue through %T", path, current)
		}
	}
	return current, nil
}

func Flatten(value any) map[string]any {
	out := map[string]any{}
	flattenInto("", value, out)
	return out
}

func Keys(value any) []string {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func Paths(value any) []string {
	var paths []string
	collectPaths("", value, &paths)
	sort.Strings(paths)
	return paths
}

func Types(value any) map[string]string {
	out := map[string]string{}
	collectTypes("", value, out)
	return out
}

func Redact(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			lowerKey := strings.ToLower(key)
			if strings.Contains(lowerKey, "token") || strings.Contains(lowerKey, "secret") || strings.Contains(lowerKey, "password") || strings.Contains(lowerKey, "key") {
				out[key] = "[redacted-secret]"
				continue
			}
			if strings.Contains(lowerKey, "email") {
				out[key] = "[redacted-email]"
				continue
			}
			out[key] = Redact(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = Redact(item)
		}
		return out
	case string:
		if strings.Contains(typed, "@") {
			return "[redacted-email]"
		}
		return typed
	default:
		return typed
	}
}

func Fingerprint(value any) (string, error) {
	shape := structuralShape(value)
	data, err := json.Marshal(shape)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func Diff(beforeReader io.Reader, afterReader io.Reader) ([]Change, error) {
	before, err := Decode(beforeReader)
	if err != nil {
		return nil, fmt.Errorf("read before JSON: %w", err)
	}
	after, err := Decode(afterReader)
	if err != nil {
		return nil, fmt.Errorf("read after JSON: %w", err)
	}

	var changes []Change
	diffValue("", before, after, &changes)
	return changes, nil
}

func diffValue(path string, before any, after any, changes *[]Change) {
	beforeMap, beforeIsMap := before.(map[string]any)
	afterMap, afterIsMap := after.(map[string]any)
	if beforeIsMap && afterIsMap {
		diffMap(path, beforeMap, afterMap, changes)
		return
	}

	if !reflect.DeepEqual(before, after) {
		*changes = append(*changes, Change{Path: path, Type: Changed, Before: before, After: after})
	}
}

func normalizeNumbers(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeNumbers(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalizeNumbers(item)
		}
		return out
	case json.Number:
		if number, err := typed.Int64(); err == nil {
			return float64(number)
		}
		number, _ := typed.Float64()
		return number
	default:
		return typed
	}
}

func flattenInto(prefix string, value any, out map[string]any) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			flattenInto(path, typed[key], out)
		}
	case []any:
		if len(typed) == 0 {
			out[prefix] = typed
			return
		}
		for i, item := range typed {
			path := strconv.Itoa(i)
			if prefix != "" {
				path = prefix + "." + path
			}
			flattenInto(path, item, out)
		}
	default:
		out[prefix] = typed
	}
}

func collectPaths(prefix string, value any, out *[]string) {
	if prefix != "" {
		*out = append(*out, prefix)
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range sortedMapKeys(typed) {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			collectPaths(path, typed[key], out)
		}
	case []any:
		for i, item := range typed {
			path := strconv.Itoa(i)
			if prefix != "" {
				path = prefix + "." + path
			}
			collectPaths(path, item, out)
		}
	}
}

func collectTypes(prefix string, value any, out map[string]string) {
	if prefix != "" {
		out[prefix] = typeName(value)
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range sortedMapKeys(typed) {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			collectTypes(path, typed[key], out)
		}
	case []any:
		for i, item := range typed {
			path := strconv.Itoa(i)
			if prefix != "" {
				path = prefix + "." + path
			}
			collectTypes(path, item, out)
		}
	}
}

func sortedMapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
	case float64:
		return "number"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func structuralShape(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = structuralShape(item)
		}
		return out
	case []any:
		shapes := make([]any, len(typed))
		for i, item := range typed {
			shapes[i] = structuralShape(item)
		}
		return shapes
	default:
		return typeName(value)
	}
}

func diffMap(prefix string, before map[string]any, after map[string]any, changes *[]Change) {
	keys := map[string]bool{}
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	sorted := make([]string, 0, len(keys))
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)

	for _, key := range sorted {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		beforeValue, beforeOK := before[key]
		afterValue, afterOK := after[key]
		switch {
		case !beforeOK:
			*changes = append(*changes, Change{Path: path, Type: Added, After: afterValue})
		case !afterOK:
			*changes = append(*changes, Change{Path: path, Type: Removed, Before: beforeValue})
		default:
			diffValue(path, beforeValue, afterValue, changes)
		}
	}
}
