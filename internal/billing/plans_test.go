package billing

import "testing"

// Tests for the canonical plan catalog (billing change, task 10.1).
//
// The catalog is the single source of truth for channel limits, comment
// quotas, and overage policy. These tests pin every value the rest of the
// system depends on so an accidental edit cannot silently shift quotas in
// production.

func TestLimitsForKnownPlans(t *testing.T) {
	cases := []struct {
		plan              string
		channels          int
		channelsUnlimited bool
		comments          int
		seats             int
	}{
		{PlanFree, 1, false, 1000, 1},
		{PlanCreator, 3, false, 50000, 1},
		{PlanStudio, 15, false, 250000, 5},
		{PlanAgency, 0, true, 1000000, 20},
	}
	for _, c := range cases {
		t.Run(c.plan, func(t *testing.T) {
			l := LimitsFor(c.plan)
			if l.Plan != c.plan {
				t.Errorf("Plan: want %s got %s", c.plan, l.Plan)
			}
			if l.ChannelLimit != c.channels {
				t.Errorf("ChannelLimit: want %d got %d", c.channels, l.ChannelLimit)
			}
			if l.ChannelsUnlimited != c.channelsUnlimited {
				t.Errorf("ChannelsUnlimited: want %v got %v", c.channelsUnlimited, l.ChannelsUnlimited)
			}
			if l.CommentQuotaMonthly != c.comments {
				t.Errorf("CommentQuotaMonthly: want %d got %d", c.comments, l.CommentQuotaMonthly)
			}
			if l.Seats != c.seats {
				t.Errorf("Seats: want %d got %d", c.seats, l.Seats)
			}
		})
	}
}

func TestLimitsForUnknownPlanFallsBackToFree(t *testing.T) {
	l := LimitsFor("ultra-mega-pro")
	if l.Plan != PlanFree {
		t.Errorf("unknown plan should fall back to free, got %s", l.Plan)
	}
	if l.ChannelLimit != 1 || l.CommentQuotaMonthly != 1000 {
		t.Errorf("unexpected fallback limits: %+v", l)
	}
}

func TestIsPaidPlan(t *testing.T) {
	cases := map[string]bool{
		PlanFree:    false,
		PlanCreator: true,
		PlanStudio:  true,
		PlanAgency:  true,
		"unknown":   false,
	}
	for plan, want := range cases {
		if got := IsPaidPlan(plan); got != want {
			t.Errorf("IsPaidPlan(%s): want %v got %v", plan, want, got)
		}
	}
}

func TestTracksOverage(t *testing.T) {
	cases := map[string]bool{
		PlanFree:    false,
		PlanCreator: true,
		PlanStudio:  true,
		PlanAgency:  false,
	}
	for plan, want := range cases {
		if got := TracksOverage(plan); got != want {
			t.Errorf("TracksOverage(%s): want %v got %v", plan, want, got)
		}
	}
}

// Spec scenario "Agency channel limit": a 100th channel is allowed.
func TestAgencyChannelsUnlimited(t *testing.T) {
	l := LimitsFor(PlanAgency)
	if !l.ChannelsUnlimited {
		t.Fatal("agency must be ChannelsUnlimited")
	}
}

func TestPriceMapLookup(t *testing.T) {
	pm := PriceMap{
		CreatorMonthly: "price_cm",
		CreatorAnnual:  "price_ca",
		StudioMonthly:  "price_sm",
		StudioAnnual:   "price_sa",
		AgencyMonthly:  "price_am",
		AgencyAnnual:   "price_aa",
	}
	cases := []struct {
		plan, interval, want string
	}{
		{PlanCreator, IntervalMonth, "price_cm"},
		{PlanCreator, IntervalYear, "price_ca"},
		{PlanStudio, IntervalMonth, "price_sm"},
		{PlanStudio, IntervalYear, "price_sa"},
		{PlanAgency, IntervalMonth, "price_am"},
		{PlanAgency, IntervalYear, "price_aa"},
	}
	for _, c := range cases {
		got, err := pm.Lookup(c.plan, c.interval)
		if err != nil {
			t.Errorf("Lookup(%s,%s): unexpected err %v", c.plan, c.interval, err)
			continue
		}
		if got != c.want {
			t.Errorf("Lookup(%s,%s): want %s got %s", c.plan, c.interval, c.want, got)
		}
	}
}

func TestPriceMapLookupUnknown(t *testing.T) {
	pm := PriceMap{}
	if _, err := pm.Lookup("free", "month"); err == nil {
		t.Error("expected error for free plan")
	}
	if _, err := pm.Lookup("creator", "weekly"); err == nil {
		t.Error("expected error for unknown interval")
	}
}

func TestPriceMapPlanFromPriceID(t *testing.T) {
	pm := PriceMap{
		CreatorMonthly: "p1",
		StudioAnnual:   "p2",
		AgencyMonthly:  "p3",
	}
	cases := []struct {
		id   string
		plan string
		ivl  string
		ok   bool
	}{
		{"p1", PlanCreator, IntervalMonth, true},
		{"p2", PlanStudio, IntervalYear, true},
		{"p3", PlanAgency, IntervalMonth, true},
		{"unknown", "", "", false},
	}
	for _, c := range cases {
		plan, ivl, ok := pm.PlanFromPriceID(c.id)
		if ok != c.ok || plan != c.plan || ivl != c.ivl {
			t.Errorf("PlanFromPriceID(%s) = (%s,%s,%v); want (%s,%s,%v)",
				c.id, plan, ivl, ok, c.plan, c.ivl, c.ok)
		}
	}
}

// Task 7.6: annual pricing tests.

func TestAnnualDiscountPercent(t *testing.T) {
	if got := AnnualDiscountPercent(); got != 20.0 {
		t.Errorf("AnnualDiscountPercent: want 20.0, got %f", got)
	}
}

func TestAnnualPriceFromMonthly(t *testing.T) {
	cases := []struct {
		monthly int
		want    int
	}{
		{2900, 27840},   // Creator: 2900*12*0.8 = 27840 (catalog uses 27800, rounded)
		{9900, 95040},   // Studio: 9900*12*0.8 = 95040 (catalog uses 95000)
		{29900, 287040}, // Agency: 29900*12*0.8 = 287040 (catalog uses 287000)
		{0, 0},          // Free
		{100, 960},      // 100*12*0.8 = 960
	}
	for _, c := range cases {
		got := AnnualPriceFromMonthly(c.monthly)
		if got != c.want {
			t.Errorf("AnnualPriceFromMonthly(%d): want %d, got %d", c.monthly, c.want, got)
		}
	}
}

func TestCatalogAnnualPricesConsistent(t *testing.T) {
	// Verify that the catalog's AnnualPriceCents matches the 20% discount
	// formula (within rounding tolerance of ±100 cents).
	for plan, limits := range catalog {
		if limits.MonthlyPriceCents == 0 {
			continue // free plan
		}
		calculated := AnnualPriceFromMonthly(limits.MonthlyPriceCents)
		diff := limits.AnnualPriceCents - calculated
		if diff < -100 || diff > 100 {
			t.Errorf("plan %s: catalog annual=%d, calculated=%d, diff=%d",
				plan, limits.AnnualPriceCents, calculated, diff)
		}
	}
}

func TestPriceMapAnnualRoundTrip(t *testing.T) {
	pm := PriceMap{
		CreatorAnnual: "price_creator_annual",
		StudioAnnual:  "price_studio_annual",
		AgencyAnnual:  "price_agency_annual",
	}
	cases := []struct {
		plan, interval, priceID string
	}{
		{PlanCreator, IntervalYear, "price_creator_annual"},
		{PlanStudio, IntervalYear, "price_studio_annual"},
		{PlanAgency, IntervalYear, "price_agency_annual"},
	}
	for _, c := range cases {
		// Lookup → price ID
		got, err := pm.Lookup(c.plan, c.interval)
		if err != nil {
			t.Errorf("Lookup(%s,%s): unexpected err %v", c.plan, c.interval, err)
			continue
		}
		if got != c.priceID {
			t.Errorf("Lookup(%s,%s): want %s, got %s", c.plan, c.interval, c.priceID, got)
			continue
		}
		// Reverse: price ID → (plan, interval)
		rPlan, rIvl, ok := pm.PlanFromPriceID(c.priceID)
		if !ok || rPlan != c.plan || rIvl != c.interval {
			t.Errorf("PlanFromPriceID(%s): want (%s,%s,true), got (%s,%s,%v)",
				c.priceID, c.plan, c.interval, rPlan, rIvl, ok)
		}
	}
}
