// Package billing owns the Stripe-backed subscription lifecycle: plan
// catalog, Checkout/Portal sessions, idempotent webhook processing,
// quota-reset cron, and plan enforcement helpers consumed by channel-
// management and llm-processing.
package billing

// Plan name constants. The values match users.subscription_plan and the
// "plan" field of POST /api/billing/checkout requests.
const (
	PlanFree    = "free"
	PlanCreator = "creator"
	PlanStudio  = "studio"
	PlanAgency  = "agency"
)

// Billing interval constants. Match users.billing_interval values.
const (
	IntervalMonth = "month"
	IntervalYear  = "year"
)

// PlanLimits is the canonical per-plan capacity record. The catalog returned
// by LimitsFor() is the single source of truth for plan-driven enforcement
// and for billing-status responses; no other file should hard-code these
// numbers.
type PlanLimits struct {
	Plan string

	// ChannelLimit is the maximum number of active+paused channels the user
	// can have. ChannelsUnlimited == true overrides this (Agency).
	ChannelLimit     int
	ChannelsUnlimited bool

	// CommentQuotaMonthly is the per-period comment processing budget.
	CommentQuotaMonthly int

	// Seats is how many users the subscription covers. v1 ignores this for
	// enforcement; it is reported through GET /api/billing/status.
	Seats int

	// MonthlyPriceCents and AnnualPriceCents are the canonical USD prices
	// rendered in the dashboard. Stripe holds the authoritative price; we
	// keep these here so the status endpoint can echo them.
	MonthlyPriceCents int
	AnnualPriceCents  int
}

// catalog is the static plan table. See spec.md "Plan Catalog".
var catalog = map[string]PlanLimits{
	PlanFree: {
		Plan:                PlanFree,
		ChannelLimit:        1,
		CommentQuotaMonthly: 1000,
		Seats:               1,
		MonthlyPriceCents:   0,
		AnnualPriceCents:    0,
	},
	PlanCreator: {
		Plan:                PlanCreator,
		ChannelLimit:        3,
		CommentQuotaMonthly: 50000,
		Seats:               1,
		MonthlyPriceCents:   2900,
		AnnualPriceCents:    27800,
	},
	PlanStudio: {
		Plan:                PlanStudio,
		ChannelLimit:        15,
		CommentQuotaMonthly: 250000,
		Seats:               5,
		MonthlyPriceCents:   9900,
		AnnualPriceCents:    95000,
	},
	PlanAgency: {
		Plan:                PlanAgency,
		ChannelsUnlimited:   true,
		ChannelLimit:        0, // unused when ChannelsUnlimited
		CommentQuotaMonthly: 1000000,
		Seats:               20,
		MonthlyPriceCents:   29900,
		AnnualPriceCents:    287000,
	},
}

// LimitsFor returns the canonical PlanLimits for the given plan name.
// Unknown plans fall back to the free-tier limits so that mis-configured
// rows never accidentally grant unbounded capacity.
func LimitsFor(plan string) PlanLimits {
	if l, ok := catalog[plan]; ok {
		return l
	}
	return catalog[PlanFree]
}

// IsPaidPlan reports whether the plan currently incurs a Stripe charge.
// Used by enforcement and overage logic.
func IsPaidPlan(plan string) bool {
	switch plan {
	case PlanCreator, PlanStudio, PlanAgency:
		return true
	}
	return false
}

// TracksOverage reports whether the system records overage rows in
// comment_usage_logs for this plan. Free is below quota by definition;
// Agency uses custom pricing handled outside this capability.
func TracksOverage(plan string) bool {
	return plan == PlanCreator || plan == PlanStudio
}
