// Package export implements streaming CSV/JSON encoders and a SQLite-cursor
// reader for the insights-api export lifecycle. It is consumed by the sync
// path of POST /api/comments/export and by the background export worker.
package export

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"noumena/internal/db"
)

// CSVHeader is the documented stable schema (specs/comment-exports). Order
// MUST NOT change without a major-version bump; downstream BI tools depend
// on it. Task 10.1, 14.5.
var CSVHeader = []string{
	"id", "channel_name", "author_handle", "body",
	"posted_at", "category", "actionable_insights",
	"topic", "confidence", "processed_at",
}

// rowValues converts a feed row into the documented CSV column order.
func rowValues(r *db.FeedRow) []string {
	authorHandle := ""
	if r.AuthorHandle != nil {
		authorHandle = *r.AuthorHandle
	}
	category := ""
	if r.Category != nil {
		category = *r.Category
	}
	actionableInsights := string(r.ActionableInsights)
	if actionableInsights == "" || actionableInsights == "null" {
		actionableInsights = "[]"
	}
	topic := ""
	if r.Topic != nil {
		topic = *r.Topic
	}
	confidence := ""
	if r.Confidence != nil {
		confidence = strconv.FormatFloat(*r.Confidence, 'f', -1, 64)
	}
	processedAt := ""
	if r.ProcessedAt != nil {
		processedAt = r.ProcessedAt.UTC().Format(time.RFC3339)
	}
	return []string{
		r.ID,
		r.ChannelName,
		authorHandle,
		r.Body,
		r.PostedAt.UTC().Format(time.RFC3339),
		category,
		actionableInsights,
		topic,
		confidence,
		processedAt,
	}
}

// EncodeCSV writes the documented header followed by one row per FeedRow.
// Encoder.Flush is invoked per row to bound memory. Task 10.1, 10.3, 10.5.
func EncodeCSV(w io.Writer, rows <-chan *db.FeedRow) (int64, error) {
	enc := csv.NewWriter(w)
	if err := enc.Write(CSVHeader); err != nil {
		return 0, err
	}
	enc.Flush()
	if err := enc.Error(); err != nil {
		return 0, err
	}
	var count int64
	for r := range rows {
		if err := enc.Write(rowValues(r)); err != nil {
			return count, err
		}
		enc.Flush()
		if err := enc.Error(); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// jsonExportRow mirrors the /feed object shape MINUS is_starred. Task 10.2.
type jsonExportRow struct {
	ID                string     `json:"id"`
	PlatformChannelID string     `json:"platform_channel_id"`
	ChannelName       string     `json:"channel_name"`
	AuthorHandle      *string    `json:"author_handle,omitempty"`
	Body              string     `json:"body"`
	PostedAt          time.Time  `json:"posted_at"`
	Category          *string    `json:"category,omitempty"`
	ActionableInsights []byte    `json:"actionable_insights,omitempty"`
	Topic             *string    `json:"topic,omitempty"`
	Confidence        *float64   `json:"confidence,omitempty"`
	ProcessedAt       *time.Time `json:"processed_at,omitempty"`
}

func toJSONRow(r *db.FeedRow) jsonExportRow {
	return jsonExportRow{
		ID:                r.ID,
		PlatformChannelID: r.PlatformChannelID,
		ChannelName:       r.ChannelName,
		AuthorHandle:      r.AuthorHandle,
		Body:              r.Body,
		PostedAt:          r.PostedAt,
		Category:          r.Category,
		ActionableInsights: r.ActionableInsights,
		Topic:             r.Topic,
		Confidence:        r.Confidence,
		ProcessedAt:       r.ProcessedAt,
	}
}

// EncodeJSON writes a single top-level array, streaming objects with
// json.Encoder so the in-memory footprint is one row at a time. Task 10.2.
func EncodeJSON(w io.Writer, rows <-chan *db.FeedRow) (int64, error) {
	if _, err := io.WriteString(w, "["); err != nil {
		return 0, err
	}
	enc := json.NewEncoder(w)
	first := true
	var count int64
	for r := range rows {
		if !first {
			if _, err := io.WriteString(w, ","); err != nil {
				return count, err
			}
		}
		first = false
		// Encoder.Encode appends a newline; acceptable inside a JSON array
		// (whitespace between values is legal). Per-row Encode flushes the
		// underlying buffered writer if w supports it implicitly.
		if err := enc.Encode(toJSONRow(r)); err != nil {
			return count, err
		}
		count++
	}
	if _, err := io.WriteString(w, "]"); err != nil {
		return count, err
	}
	return count, nil
}

// StreamExport opens a SQLite query with BuildFeedQuery(ModeStream),
// fans rows into a channel, and dispatches to the requested encoder. The
// row channel decouples the DB cursor from the encoder so the encoder can
// flush before the next row is fetched, bounding memory. Task 10.4.
func StreamExport(ctx context.Context, pool *sql.DB, userID string, filters db.FeedFilters, format string, dst io.Writer) (int64, error) {
	sqlQuery, args := db.BuildFeedQuery(userID, filters, db.ModeStream)
	rows, err := pool.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	rowCh := make(chan *db.FeedRow, 64)
	errCh := make(chan error, 1)
	go func() {
		defer close(rowCh)
		for rows.Next() {
			r := &db.FeedRow{}
			if err := rows.Scan(
				&r.ID, &r.PlatformChannelID, &r.PlatformCommentID, &r.ParentCommentID,
				&r.AuthorHandle, &r.Body, &r.PostedAt, &r.Category, &r.ActionableInsights,
				&r.Topic, &r.Confidence, &r.ProcessedAt, &r.CreatedAt,
				&r.ChannelName, &r.IsStarred,
			); err != nil {
				errCh <- err
				return
			}
			select {
			case rowCh <- r:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}
		errCh <- rows.Err()
	}()

	var (
		count   int64
		encErr  error
	)
	switch format {
	case "csv":
		count, encErr = EncodeCSV(dst, rowCh)
	case "json":
		count, encErr = EncodeJSON(dst, rowCh)
	default:
		return 0, fmt.Errorf("unknown format %q", format)
	}
	// Drain any pending scan errors.
	for range rowCh {
		// rowCh may already be closed; ignored
	}
	if scanErr := <-errCh; scanErr != nil && encErr == nil {
		return count, scanErr
	}
	return count, encErr
}
