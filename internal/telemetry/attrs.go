// Package telemetry centralises OpenTelemetry initialisation and the
// closed-enum attribute values that subsystem call sites pass to
// instruments. Centralising the enums here is the cardinality firewall:
// a stray free-form error string can never reach the metrics backend.
package telemetry

import "go.opentelemetry.io/otel/attribute"

// Closed-enum attribute keys. Defined as constants (not strings inlined
// at call sites) so a `git grep` finds every recording site for a given
// attribute and the spec contract stays auditable.
const (
	AttrReason       = "reason"
	AttrOutcome      = "outcome"
	AttrEvent        = "event"
	AttrPlatform     = "platform"
	AttrProvider     = "provider"
	AttrModel        = "model"
	AttrFromProvider = "from_provider"
	AttrToProvider   = "to_provider"
	AttrTemplate     = "template"
	AttrMethod       = "method"
	AttrRoute        = "route"
	AttrStatus       = "status"
	AttrRetryable    = "retryable"
)

// Reason enum (ingestion errors). The full spec list is mirrored here so
// callers can pass `telemetry.ReasonRateLimited` instead of the bare
// string "rate_limited".
const (
	ReasonRateLimited      = "rate_limited"
	ReasonTokenInvalid     = "token_invalid"
	ReasonDeadlineExceeded = "deadline_exceeded"
	ReasonDeadLettered     = "dead_lettered"
	ReasonPanic            = "panic"
	ReasonOther            = "other"
)

// Outcome enum (LLM, ingestion-fetch, lifecycle, email, http).
// `Aborted` and `IdempotentDedupe` are subsystem-specific extensions of
// the base success/error/timeout vocabulary; the spec lists them per
// instrument so we keep them here as named constants too.
const (
	OutcomeSuccess           = "success"
	OutcomeError             = "error"
	OutcomeTimeout           = "timeout"
	OutcomeAbortedNewTracker = "aborted_new_tracker"
	OutcomeSignatureInvalid  = "signature_invalid"
	OutcomeHandlerError      = "handler_error"
	OutcomeIgnored           = "ignored"
	OutcomeIdempotentDedupe  = "idempotent_dedupe"
	// LLM job-attempt outcomes (fix-llm-duplicate-processing).
	OutcomeCompleted       = "completed"
	OutcomeFailedTransient = "failed_transient"
	OutcomeFailedTerminal  = "failed_terminal"
	OutcomeParked          = "parked"
)

// Platform enum (ingestion). Only "youtube" today; reserved as an enum
// so adding TikTok / Twitch later is a one-line constant addition.
const (
	PlatformYouTube = "youtube"
)

// validReasons / validOutcomes back the test-only validators below.
var validReasons = map[string]struct{}{
	ReasonRateLimited:      {},
	ReasonTokenInvalid:     {},
	ReasonDeadlineExceeded: {},
	ReasonDeadLettered:     {},
	ReasonPanic:            {},
	ReasonOther:            {},
}

var validOutcomes = map[string]struct{}{
	OutcomeSuccess:           {},
	OutcomeError:             {},
	OutcomeTimeout:           {},
	OutcomeAbortedNewTracker: {},
	OutcomeSignatureInvalid:  {},
	OutcomeHandlerError:      {},
	OutcomeIgnored:           {},
	OutcomeIdempotentDedupe:  {},
	OutcomeCompleted:         {},
	OutcomeFailedTransient:   {},
	OutcomeFailedTerminal:    {},
	OutcomeParked:            {},
}

// Reason returns an attribute.KeyValue with a validated reason. In
// production an unknown value is silently coerced to "other" rather than
// panicking — losing label fidelity is preferable to crashing the
// process. Tests can use ValidReason to assert the call site at compile
// time of the test binary.
func Reason(v string) attribute.KeyValue {
	if _, ok := validReasons[v]; !ok {
		v = ReasonOther
	}
	return attribute.String(AttrReason, v)
}

// Outcome mirrors Reason for outcome enums.
func Outcome(v string) attribute.KeyValue {
	if _, ok := validOutcomes[v]; !ok {
		v = OutcomeError
	}
	return attribute.String(AttrOutcome, v)
}

// ValidReason / ValidOutcome are exported for tests under _test.go files
// that want to assert "every code path emits a known label".
func ValidReason(v string) bool  { _, ok := validReasons[v]; return ok }
func ValidOutcome(v string) bool { _, ok := validOutcomes[v]; return ok }
