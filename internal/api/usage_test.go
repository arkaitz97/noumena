package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"noumena/internal/auth"
	"noumena/internal/billing"
	"noumena/internal/db"
)

// Tests for implement-missing-product-features billing additions:
// 3.2 GET /api/billing/usage and 5.2 trial derivation in
// GET /api/billing/status.

// ============================================================================
// 3.2 GET /api/billing/usage
// ============================================================================

func TestBillingUsageReturnsNewestFirst(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "test-user-id", billing.PlanCreator)
	now := time.Now().UTC()
	queries.billingUsage = map[string][]db.BillingUsage{
		"test-user-id": {
			{UserID: "test-user-id", PeriodStart: now.AddDate(0, -1, 0), PeriodEnd: now, CommentsUsed: 12, Quota: 50},
			{UserID: "test-user-id", PeriodStart: now.AddDate(0, -2, 0), PeriodEnd: now.AddDate(0, -1, 0), CommentsUsed: 40, Quota: 50},
			{UserID: "test-user-id", PeriodStart: now.AddDate(0, -3, 0), PeriodEnd: now.AddDate(0, -2, 0), CommentsUsed: 9, Quota: 50},
		},
	}
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/usage?periods=2", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Periods []struct {
			PeriodStart  string `json:"period_start"`
			PeriodEnd    string `json:"period_end"`
			CommentsUsed int    `json:"comments_used"`
			Quota        int    `json:"quota"`
		} `json:"periods"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Periods) != 2 {
		t.Fatalf("periods=2: want 2 got %d", len(body.Periods))
	}
	if body.Periods[0].CommentsUsed != 12 {
		t.Errorf("newest first: want comments_used 12 got %d", body.Periods[0].CommentsUsed)
	}
	if body.Periods[1].CommentsUsed != 40 {
		t.Errorf("second period: want comments_used 40 got %d", body.Periods[1].CommentsUsed)
	}
	if body.Periods[0].Quota != 50 {
		t.Errorf("quota: want 50 got %d", body.Periods[0].Quota)
	}
	if body.Periods[0].PeriodStart == "" || body.Periods[0].PeriodEnd == "" {
		t.Error("period timestamps must be present")
	}
}

func TestBillingUsageDefaultSix(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "test-user-id", billing.PlanCreator)
	now := time.Now().UTC()
	rows := make([]db.BillingUsage, 0, 8)
	for i := 0; i < 8; i++ {
		rows = append(rows, db.BillingUsage{
			UserID: "test-user-id", PeriodStart: now.AddDate(0, -i-1, 0),
			PeriodEnd: now.AddDate(0, -i, 0), CommentsUsed: i, Quota: 50,
		})
	}
	queries.billingUsage = map[string][]db.BillingUsage{"test-user-id": rows}
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/usage", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Periods []struct{ CommentsUsed int } `json:"periods"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Periods) != 6 {
		t.Errorf("default periods: want 6 got %d", len(body.Periods))
	}
}

func TestBillingUsageInvalidPeriodsParam(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "test-user-id", billing.PlanFree)
	r := setupBillingRouter(queries, billing.NewMockClient())
	access, _, _ := auth.GenerateTokens("test-user-id")

	for _, q := range []string{"0", "-1", "13", "abc", "6.5"} {
		req, _ := http.NewRequest("GET", "/api/billing/usage?periods="+q, nil)
		req.Header.Set("Authorization", bearer(access))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("periods=%s: want 400 got %d", q, rr.Code)
		}
	}
}

func TestBillingUsageMaxClamp(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "test-user-id", billing.PlanFree)
	r := setupBillingRouter(queries, billing.NewMockClient())
	access, _, _ := auth.GenerateTokens("test-user-id")

	req, _ := http.NewRequest("GET", "/api/billing/usage?periods=12", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("periods=12: want 200 got %d", rr.Code)
	}
}

func TestBillingUsageUnauthenticated(t *testing.T) {
	r := setupBillingRouter(&MockQuery{}, billing.NewMockClient())
	req, _ := http.NewRequest("GET", "/api/billing/usage", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401 got %d", rr.Code)
	}
}

// ============================================================================
// 5.2 trial derivation in GET /api/billing/status
// ============================================================================

func TestBillingStatusActiveTrial(t *testing.T) {
	queries := &MockQuery{}
	u := seedTestUser(queries, "test-user-id", billing.PlanFree)
	endsAt := time.Now().Add(10 * 24 * time.Hour).UTC()
	u.TrialEndsAt = &endsAt
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/status", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
	var body struct {
		Trial struct {
			Active        bool   `json:"active"`
			EndsAt        string `json:"ends_at"`
			DaysRemaining int    `json:"days_remaining"`
		} `json:"trial"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if !body.Trial.Active {
		t.Error("trial.active: want true for future trial_ends_at on free plan")
	}
	if body.Trial.DaysRemaining != 10 {
		t.Errorf("trial.days_remaining: want 10 got %d", body.Trial.DaysRemaining)
	}
	if body.Trial.EndsAt == "" {
		t.Error("trial.ends_at: want RFC3339 timestamp")
	}
}

func TestBillingStatusExpiredTrial(t *testing.T) {
	queries := &MockQuery{}
	u := seedTestUser(queries, "test-user-id", billing.PlanFree)
	past := time.Now().Add(-3 * 24 * time.Hour).UTC()
	u.TrialEndsAt = &past
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/status", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var body struct {
		Trial struct {
			Active        bool `json:"active"`
			DaysRemaining int  `json:"days_remaining"`
		} `json:"trial"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.Trial.Active {
		t.Error("trial.active: want false for expired trial")
	}
	if body.Trial.DaysRemaining != 0 {
		t.Errorf("trial.days_remaining: want 0 got %d", body.Trial.DaysRemaining)
	}
}

func TestBillingStatusPaidPlanNotTrial(t *testing.T) {
	queries := &MockQuery{}
	u := seedTestUser(queries, "test-user-id", billing.PlanCreator)
	endsAt := time.Now().Add(10 * 24 * time.Hour).UTC()
	u.TrialEndsAt = &endsAt
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/status", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var body struct {
		Trial struct {
			Active bool `json:"active"`
		} `json:"trial"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.Trial.Active {
		t.Error("trial.active: want false on paid plan despite future trial_ends_at")
	}
}

func TestBillingStatusNoTrialFields(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "test-user-id", billing.PlanFree)
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/status", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var body struct {
		Trial struct {
			Active bool   `json:"active"`
			EndsAt string `json:"ends_at"`
		} `json:"trial"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.Trial.Active {
		t.Error("trial.active: want false when no trial_ends_at")
	}
	if body.Trial.EndsAt != "" {
		t.Errorf("trial.ends_at: want empty when no trial_ends_at, got %q", body.Trial.EndsAt)
	}
}

// ============================================================================
// llm-cost-analytics 2.4: cost fields on usage + status
// ============================================================================

func TestBillingUsageIncludesLLMCost(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "test-user-id", billing.PlanCreator)
	now := time.Now().UTC()
	queries.billingUsage = map[string][]db.BillingUsage{
		"test-user-id": {
			{UserID: "test-user-id", PeriodStart: now.AddDate(0, -1, 0), PeriodEnd: now, CommentsUsed: 12, Quota: 50, LLMCostUSDMicro: 1_234_500, PriceVersion: 1},
			{UserID: "test-user-id", PeriodStart: now.AddDate(0, -2, 0), PeriodEnd: now.AddDate(0, -1, 0), CommentsUsed: 40, Quota: 50},
		},
	}
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/usage", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Periods []struct {
			CommentsUsed int     `json:"comments_used"`
			LLMCostUSD   float64 `json:"llm_cost_usd"`
		} `json:"periods"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Periods) != 2 {
		t.Fatalf("want 2 periods got %d", len(body.Periods))
	}
	if got := body.Periods[0].LLMCostUSD; got != 1.2345 {
		t.Errorf("llm_cost_usd: want 1.2345 (micro→USD at boundary) got %v", got)
	}
	if got := body.Periods[1].LLMCostUSD; got != 0 {
		t.Errorf("absent cost row: want 0 got %v", got)
	}
}

func TestBillingStatusIncludesLLMCostMTD(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "test-user-id", billing.PlanCreator)
	now := time.Now().UTC()
	queries.billingUsage = map[string][]db.BillingUsage{
		"test-user-id": {
			{UserID: "test-user-id", PeriodStart: now.AddDate(0, 0, -10), PeriodEnd: now.AddDate(0, 0, 20), CommentsUsed: 12, Quota: 50, LLMCostUSDMicro: 456_789, PriceVersion: 1},
			{UserID: "test-user-id", PeriodStart: now.AddDate(0, -1, -10), PeriodEnd: now.AddDate(0, 0, -10), CommentsUsed: 40, Quota: 50, LLMCostUSDMicro: 9_999_999},
		},
	}
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/status", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
	var body struct {
		LLMCostMTDUSD float64 `json:"llm_cost_mtd_usd"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.LLMCostMTDUSD != 0.456789 {
		t.Errorf("llm_cost_mtd_usd: want current-period row 0.456789 got %v", body.LLMCostMTDUSD)
	}
}

func TestBillingStatusLLMCostMTDAbsentIsZero(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "test-user-id", billing.PlanCreator)
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/status", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var body struct {
		LLMCostMTDUSD *float64 `json:"llm_cost_mtd_usd"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.LLMCostMTDUSD == nil {
		t.Fatal("llm_cost_mtd_usd must always be present (additive field)")
	}
	if *body.LLMCostMTDUSD != 0 {
		t.Errorf("no rollup rows: want 0 got %v", *body.LLMCostMTDUSD)
	}
}
