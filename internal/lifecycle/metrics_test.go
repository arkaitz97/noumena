package lifecycle

import (
	"context"
	"testing"

	"noumena/internal/telemetry"
)

// Task 8.6: a chunked deletion of 3 chunks records 3 chunk_rows
// observations and 1 success completion. We exercise the record
// helpers directly rather than the worker because the worker requires
// a full DBQuery surface; the helpers ARE the contract surface and
// trivially compose into the worker via straight-line code.
func TestDeletionChunkedRecording(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	ctx := context.Background()
	// Three chunks of 500/500/123 rows — typical pattern when the
	// last chunk is short.
	recordDeletionChunkRows(ctx, 500)
	recordDeletionChunkRows(ctx, 500)
	recordDeletionChunkRows(ctx, 123)
	recordDeletionCompleted(ctx, telemetry.OutcomeSuccess)

	var rm = collect(t, reader)

	count, sum := histCountWithAttr(&rm, "noumena_deletion_chunk_rows", "", "")
	if count != 3 {
		t.Errorf("chunk_rows count: want 3, got %d", count)
	}
	if sum != 1123 {
		t.Errorf("chunk_rows sum: want 1123, got %d", sum)
	}

	if got := sumInt64WithAttr(&rm, "noumena_deletions_completed_total",
		telemetry.AttrOutcome, telemetry.OutcomeSuccess); got != 1 {
		t.Errorf("deletions_completed_total{success}: want 1, got %d", got)
	}
}

func TestDeletionAbortedNewTrackerCounter(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	ctx := context.Background()
	// Simulate the race: 2 chunks landed, then a new tracker
	// appeared and we bailed mid-channel.
	recordDeletionChunkRows(ctx, 500)
	recordDeletionChunkRows(ctx, 200)
	recordDeletionCompleted(ctx, telemetry.OutcomeAbortedNewTracker)

	rm := collect(t, reader)
	if got := sumInt64WithAttr(&rm, "noumena_deletions_completed_total",
		telemetry.AttrOutcome, telemetry.OutcomeAbortedNewTracker); got != 1 {
		t.Errorf("deletions_completed_total{aborted_new_tracker}: want 1, got %d", got)
	}
}

func TestDeletionFailureCounter(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	ctx := context.Background()
	recordDeletionCompleted(ctx, telemetry.OutcomeError)

	rm := collect(t, reader)
	if got := sumInt64WithAttr(&rm, "noumena_deletions_completed_total",
		telemetry.AttrOutcome, telemetry.OutcomeError); got != 1 {
		t.Errorf("deletions_completed_total{error}: want 1, got %d", got)
	}
}

func TestExportSuccessRecordsBytes(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	ctx := context.Background()
	recordExportBytes(ctx, 4096)
	recordExportCompleted(ctx, telemetry.OutcomeSuccess)

	rm := collect(t, reader)
	if got := sumInt64Total(&rm, "noumena_export_bytes_total"); got != 4096 {
		t.Errorf("export_bytes_total: want 4096, got %d", got)
	}
	if got := sumInt64WithAttr(&rm, "noumena_exports_completed_total",
		telemetry.AttrOutcome, telemetry.OutcomeSuccess); got != 1 {
		t.Errorf("exports_completed_total{success}: want 1, got %d", got)
	}
}

func TestExportFailureSkipsBytes(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	ctx := context.Background()
	// On failure path the worker calls recordExportCompleted(error)
	// WITHOUT recordExportBytes — the half-written ZIP is unlinked.
	recordExportCompleted(ctx, telemetry.OutcomeError)

	rm := collect(t, reader)
	if got := sumInt64Total(&rm, "noumena_export_bytes_total"); got != 0 {
		t.Errorf("export_bytes_total on failure: want 0, got %d", got)
	}
	if got := sumInt64WithAttr(&rm, "noumena_exports_completed_total",
		telemetry.AttrOutcome, telemetry.OutcomeError); got != 1 {
		t.Errorf("exports_completed_total{error}: want 1, got %d", got)
	}
}

// recordExportBytes guards against zero/negative inputs (e.g. failed
// os.Stat returning a zero-value FileInfo). Verify both branches.
func TestExportBytesIgnoresNonPositive(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	ctx := context.Background()
	recordExportBytes(ctx, 0)
	recordExportBytes(ctx, -1)

	rm := collect(t, reader)
	if got := sumInt64Total(&rm, "noumena_export_bytes_total"); got != 0 {
		t.Errorf("export_bytes_total non-positive: want 0, got %d", got)
	}
}
