package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Format string

const (
	Text     Format = "text"
	JSON     Format = "json"
	CSV      Format = "csv"
	Markdown Format = "markdown"
	Raw      Format = "raw"
)

type Options struct {
	Format Format
	Quiet  bool
}

type Record map[string]any

func Render(options Options, value any) (string, error) {
	switch options.Format {
	case JSON:
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data) + "\n", nil
	case CSV:
		records, ok := value.([]Record)
		if !ok {
			return "", fmt.Errorf("csv output requires records")
		}
		return renderCSV(records)
	case Markdown:
		records, ok := value.([]Record)
		if !ok {
			return "", fmt.Errorf("markdown output requires records")
		}
		return renderMarkdown(records), nil
	case Raw:
		text := ""
		switch typed := value.(type) {
		case string:
			text = typed
		case []byte:
			text = string(typed)
		default:
			data, err := json.MarshalIndent(value, "", "  ")
			if err != nil {
				return "", err
			}
			text = string(data)
		}
		if strings.HasSuffix(text, "\n") {
			return text, nil
		}
		return text + "\n", nil
	default:
		switch v := value.(type) {
		case []Record:
			return renderText(v), nil
		case string:
			return v, nil
		default:
			data, err := json.MarshalIndent(value, "", "  ")
			if err != nil {
				return "", err
			}
			return string(data) + "\n", nil
		}
	}
}

func renderCSV(records []Record) (string, error) {
	if len(records) == 0 {
		return "", nil
	}
	headers := sortedHeaders(records)
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(headers); err != nil {
		return "", err
	}
	for _, record := range records {
		row := make([]string, len(headers))
		for i, header := range headers {
			row[i] = fmt.Sprint(record[header])
		}
		if err := writer.Write(row); err != nil {
			return "", err
		}
	}
	writer.Flush()
	return buf.String(), writer.Error()
}

func renderMarkdown(records []Record) string {
	if len(records) == 0 {
		return ""
	}
	headers := sortedHeaders(records)
	var b strings.Builder
	b.WriteString("| ")
	b.WriteString(strings.Join(headers, " | "))
	b.WriteString(" |\n| ")
	for i := range headers {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString("---")
	}
	b.WriteString(" |\n")
	for _, record := range records {
		b.WriteString("| ")
		for i, header := range headers {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(fmt.Sprint(record[header]))
		}
		b.WriteString(" |\n")
	}
	return b.String()
}

func renderText(records []Record) string {
	if len(records) == 0 {
		return ""
	}
	headers := sortedHeaders(records)
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, record := range records {
		for i, header := range headers {
			widths[i] = max(widths[i], len(fmt.Sprint(record[header])))
		}
	}

	var b strings.Builder
	for i, header := range headers {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(pad(strings.ToUpper(header), widths[i]))
	}
	b.WriteByte('\n')
	for i, width := range widths {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(strings.Repeat("-", width))
	}
	b.WriteByte('\n')
	for _, record := range records {
		for i, header := range headers {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(pad(fmt.Sprint(record[header]), widths[i]))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func sortedHeaders(records []Record) []string {
	seen := map[string]bool{}
	for _, record := range records {
		for key := range record {
			seen[key] = true
		}
	}
	headers := make([]string, 0, len(seen))
	for key := range seen {
		headers = append(headers, key)
	}
	sort.Strings(headers)
	return headers
}

func pad(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}
