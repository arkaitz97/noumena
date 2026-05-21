package export

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"noumena/internal/db"
)

func makeRow(id, body string) *db.FeedRow {
	cat := "question"
	topic := "lighting"
	conf := 0.85
	posted := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	processed := posted.Add(time.Minute)
	return &db.FeedRow{
		Comment: db.Comment{
			ID:                id,
			PlatformChannelID: "ch1",
			PlatformCommentID: "yt-" + id,
			Body:              body,
			PostedAt:          posted,
			Category:          &cat,
			ActionableInsights: []byte(`[]`),
			Topic:             &topic,
			Confidence:        &conf,
			ProcessedAt:       &processed,
			CreatedAt:         posted,
		},
		ChannelName: "My Channel",
	}
}

func feed(rows ...*db.FeedRow) <-chan *db.FeedRow {
	ch := make(chan *db.FeedRow, len(rows))
	for _, r := range rows {
		ch <- r
	}
	close(ch)
	return ch
}

func TestEncodeCSV_HeaderOrder(t *testing.T) {
	var buf bytes.Buffer
	if _, err := EncodeCSV(&buf, feed()); err != nil {
		t.Fatalf("encode: %v", err)
	}
	r := csv.NewReader(&buf)
	header, err := r.Read()
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	want := []string{"id", "channel_name", "author_handle", "body", "posted_at", "category", "actionable_insights", "topic", "confidence", "processed_at"}
	if len(header) != len(want) {
		t.Fatalf("header length: got %d want %d", len(header), len(want))
	}
	for i, w := range want {
		if header[i] != w {
			t.Errorf("header[%d]: got %q want %q", i, header[i], w)
		}
	}
}

func TestEncodeCSV_EscapesQuotesCommasNewlines(t *testing.T) {
	row := makeRow("c1", `quoted "value", with comma
and newline`)
	var buf bytes.Buffer
	if _, err := EncodeCSV(&buf, feed(row)); err != nil {
		t.Fatalf("encode: %v", err)
	}
	r := csv.NewReader(&buf)
	r.FieldsPerRecord = -1
	all, err := r.ReadAll()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected header+1, got %d", len(all))
	}
	got := all[1][3] // body column
	if !strings.Contains(got, "quoted \"value\", with comma") {
		t.Errorf("body not preserved: %q", got)
	}
	if !strings.Contains(got, "newline") {
		t.Errorf("newline body not preserved: %q", got)
	}
}

func TestEncodeJSON_OmitsIsStarredAndStreamsArray(t *testing.T) {
	r := makeRow("c1", "hello")
	r.IsStarred = true // must NOT appear in output
	var buf bytes.Buffer
	if _, err := EncodeJSON(&buf, feed(r)); err != nil {
		t.Fatalf("encode: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("not a top-level array: %q", out)
	}
	if strings.Contains(out, "is_starred") {
		t.Errorf("is_starred leaked into JSON export: %q", out)
	}
	// Validate array shape.
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 row, got %d", len(arr))
	}
}

func TestEncodeBoth_ZeroRows(t *testing.T) {
	var buf bytes.Buffer
	n, err := EncodeCSV(&buf, feed())
	if err != nil {
		t.Fatalf("csv: %v", err)
	}
	if n != 0 {
		t.Errorf("csv row count: %d", n)
	}
	buf.Reset()
	n, err = EncodeJSON(&buf, feed())
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	if n != 0 {
		t.Errorf("json row count: %d", n)
	}
	if strings.TrimSpace(buf.String()) != "[]" {
		t.Errorf("zero-row JSON should be []: %q", buf.String())
	}
}
