package llm

import (
	"context"
	"log"
	"strconv"
	"time"

	"noumena/internal/db"
)

// llmCostStore is the optional DB surface for cost accumulation. Reached
// by type assertion (the DBQuery interface stays unchanged so existing
// test fakes compile; embedded-nil doubles recover to a no-op, mirroring
// incrementCommentsUsed).
type llmCostStore interface {
	AddLLMCost(ctx context.Context, userID string, periodStart, periodEnd time.Time, costUSDMicro int64, priceVersion int) error
}

// persistLLMCost accumulates costMicro onto the user's billing-period
// rollup. Failures are logged, never fatal: the classification results
// are already persisted and a lost cost row is a display gap, not a
// correctness bug. Zero cost is a no-op (unreported usage never writes
// rows so absent truly means zero).
func persistLLMCost(ctx context.Context, q db.DBQuery, userID string, periodStart, periodEnd time.Time, costMicro int64) {
	if costMicro <= 0 {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			// Nil-embedded db.DBQuery test double: no cost store.
			log.Printf("LLM cost: persist panic user=%s: %v", userID, r)
		}
	}()
	store, ok := q.(llmCostStore)
	if !ok {
		return
	}
	if err := store.AddLLMCost(ctx, userID, periodStart, periodEnd, costMicro, PriceVersion); err != nil {
		log.Printf("LLM cost: accumulate failed user=%s: %v", userID, err)
	}
}

// recordCost emits the per-tenant ops counter (labels: user, channel,
// price_version). channelID may be empty (digest costs are account-wide).
func recordCost(ctx context.Context, userID, channelID string, costMicro int64) {
	recordLLMCost(ctx, userID, channelID, strconv.Itoa(PriceVersion), costMicro)
}
