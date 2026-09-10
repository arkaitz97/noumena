package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"noumena/internal/db"
)

type ProfileHandler struct {
	Queries db.DBQuery
}

var validUserTypes = map[string]bool{
	"creator": true,
	"agency":  true,
	"brand":   true,
	"other":   true,
}

var validPrimaryGoals = map[string]bool{
	"understand_audience":    true,
	"generate_content_ideas": true,
	"improve_retention":      true,
	"win_sponsorships":       true,
	"reduce_toxicity":        true,
	"report_to_clients":      true,
	"other":                  true,
}

var validPrimaryPlatforms = map[string]bool{
	"youtube":   true,
	"tiktok":    true,
	"instagram": true,
	"other":     true,
}

var validAudienceSizeBuckets = map[string]bool{
	"under_1k":          true,
	"1k_10k":            true,
	"10k_50k":           true,
	"50k_500k":          true,
	"500k_1m":           true,
	"over_1m":           true,
	"prefer_not_to_say": true,
}

var validCommentVolumeBuckets = map[string]bool{
	"under_100":         true,
	"100_1k":            true,
	"1k_10k":            true,
	"10k_100k":          true,
	"over_100k":         true,
	"prefer_not_to_say": true,
}

var validSuccessMetrics = map[string]bool{
	"views":          true,
	"watch_time":     true,
	"subscribers":    true,
	"engagement":     true,
	"sentiment":      true,
	"sponsor_deals":  true,
	"product_sales":  true,
	"client_reports": true,
	"other":          true,
}

var validRoles = map[string]bool{
	"owner":      true,
	"strategist": true,
	"analyst":    true,
	"moderator":  true,
	"viewer":     true,
	"other":      true,
}

var validTeamSizeBuckets = map[string]bool{
	"solo":              true,
	"2_5":               true,
	"6_20":              true,
	"21_plus":           true,
	"prefer_not_to_say": true,
}

var validPublishingFrequencyBuckets = map[string]bool{
	"daily":             true,
	"several_weekly":    true,
	"weekly":            true,
	"biweekly":          true,
	"monthly":           true,
	"irregular":         true,
	"prefer_not_to_say": true,
}

var validInsightPriorities = map[string]bool{
	"questions":     true,
	"content_ideas": true,
	"criticism":     true,
	"toxicity":      true,
	"collaboration": true,
	"sentiment":     true,
}

var validCurrentWorkflows = map[string]bool{
	"native_tools":     true,
	"spreadsheets":     true,
	"moderation_tool":  true,
	"agency_dashboard": true,
	"none":             true,
	"other":            true,
}

var validBiggestPains = map[string]bool{
	"too_many_comments":    true,
	"missed_feedback":      true,
	"toxicity":             true,
	"reporting_time":       true,
	"unclear_content_plan": true,
	"team_visibility":      true,
	"other":                true,
}

type patchProfileRequest struct {
	DisplayName               *string   `json:"display_name"`
	UserType                  *string   `json:"user_type"`
	PrimaryGoal               *string   `json:"primary_goal"`
	PrimaryPlatform           *string   `json:"primary_platform"`
	ContentTopics             *[]string `json:"content_topics"`
	AudienceSizeBucket        *string   `json:"audience_size_bucket"`
	WeeklyCommentVolumeBucket *string   `json:"weekly_comment_volume_bucket"`
	SuccessMetric             *string   `json:"success_metric"`
	Role                      *string   `json:"role"`
	TeamSizeBucket            *string   `json:"team_size_bucket"`
	PublishingFrequencyBucket *string   `json:"publishing_frequency_bucket"`
	InsightPriorities         *[]string `json:"insight_priorities"`
	CurrentWorkflow           *string   `json:"current_workflow"`
	BiggestPain               *string   `json:"biggest_pain"`
}

func (h *ProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	p, err := h.Queries.GetUserProfile(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}

	if p == nil {
		// Return empty default profile for users without a saved profile record
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id":                      userID,
			"display_name":                 nil,
			"user_type":                    nil,
			"primary_goal":                 nil,
			"primary_platform":             nil,
			"content_topics":               []string{},
			"audience_size_bucket":         nil,
			"weekly_comment_volume_bucket": nil,
			"success_metric":               nil,
			"role":                         nil,
			"team_size_bucket":             nil,
			"publishing_frequency_bucket":  nil,
			"insight_priorities":           []string{},
			"current_workflow":             nil,
			"biggest_pain":                 nil,
			"created_at":                   nil,
			"updated_at":                   nil,
		})
		return
	}

	if p.ContentTopics == nil {
		p.ContentTopics = []string{}
	}
	if p.InsightPriorities == nil {
		p.InsightPriorities = []string{}
	}

	writeJSON(w, http.StatusOK, p)
}

func cleanOptionalString(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (h *ProfileHandler) PatchProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req patchProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var (
		cleanedUserType                  *string
		cleanedPrimaryGoal               *string
		cleanedPrimaryPlatform           *string
		cleanedAudienceSizeBucket        *string
		cleanedWeeklyCommentVolumeBucket *string
		cleanedSuccessMetric             *string
		cleanedRole                      *string
		cleanedTeamSizeBucket            *string
		cleanedPublishingFrequencyBucket *string
		cleanedCurrentWorkflow           *string
		cleanedBiggestPain               *string
	)

	// Validate fields
	if req.DisplayName != nil {
		trimmed := strings.TrimSpace(*req.DisplayName)
		if len(trimmed) > 100 {
			writeError(w, http.StatusBadRequest, "display_name exceeds maximum length of 100 characters")
			return
		}
	}
	if req.UserType != nil {
		cleanedUserType = cleanOptionalString(req.UserType)
		if cleanedUserType != nil && !validUserTypes[*cleanedUserType] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid user_type: %s", *cleanedUserType))
			return
		}
	}
	if req.PrimaryGoal != nil {
		cleanedPrimaryGoal = cleanOptionalString(req.PrimaryGoal)
		if cleanedPrimaryGoal != nil && !validPrimaryGoals[*cleanedPrimaryGoal] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid primary_goal: %s", *cleanedPrimaryGoal))
			return
		}
	}
	if req.PrimaryPlatform != nil {
		cleanedPrimaryPlatform = cleanOptionalString(req.PrimaryPlatform)
		if cleanedPrimaryPlatform != nil && !validPrimaryPlatforms[*cleanedPrimaryPlatform] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid primary_platform: %s", *cleanedPrimaryPlatform))
			return
		}
	}
	if req.AudienceSizeBucket != nil {
		cleanedAudienceSizeBucket = cleanOptionalString(req.AudienceSizeBucket)
		if cleanedAudienceSizeBucket != nil && !validAudienceSizeBuckets[*cleanedAudienceSizeBucket] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid audience_size_bucket: %s", *cleanedAudienceSizeBucket))
			return
		}
	}
	if req.WeeklyCommentVolumeBucket != nil {
		cleanedWeeklyCommentVolumeBucket = cleanOptionalString(req.WeeklyCommentVolumeBucket)
		if cleanedWeeklyCommentVolumeBucket != nil && !validCommentVolumeBuckets[*cleanedWeeklyCommentVolumeBucket] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid weekly_comment_volume_bucket: %s", *cleanedWeeklyCommentVolumeBucket))
			return
		}
	}
	if req.SuccessMetric != nil {
		cleanedSuccessMetric = cleanOptionalString(req.SuccessMetric)
		if cleanedSuccessMetric != nil && !validSuccessMetrics[*cleanedSuccessMetric] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid success_metric: %s", *cleanedSuccessMetric))
			return
		}
	}
	if req.Role != nil {
		cleanedRole = cleanOptionalString(req.Role)
		if cleanedRole != nil && !validRoles[*cleanedRole] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid role: %s", *cleanedRole))
			return
		}
	}
	if req.TeamSizeBucket != nil {
		cleanedTeamSizeBucket = cleanOptionalString(req.TeamSizeBucket)
		if cleanedTeamSizeBucket != nil && !validTeamSizeBuckets[*cleanedTeamSizeBucket] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid team_size_bucket: %s", *cleanedTeamSizeBucket))
			return
		}
	}
	if req.PublishingFrequencyBucket != nil {
		cleanedPublishingFrequencyBucket = cleanOptionalString(req.PublishingFrequencyBucket)
		if cleanedPublishingFrequencyBucket != nil && !validPublishingFrequencyBuckets[*cleanedPublishingFrequencyBucket] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid publishing_frequency_bucket: %s", *cleanedPublishingFrequencyBucket))
			return
		}
	}
	if req.CurrentWorkflow != nil {
		cleanedCurrentWorkflow = cleanOptionalString(req.CurrentWorkflow)
		if cleanedCurrentWorkflow != nil && !validCurrentWorkflows[*cleanedCurrentWorkflow] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid current_workflow: %s", *cleanedCurrentWorkflow))
			return
		}
	}
	if req.BiggestPain != nil {
		cleanedBiggestPain = cleanOptionalString(req.BiggestPain)
		if cleanedBiggestPain != nil && !validBiggestPains[*cleanedBiggestPain] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid biggest_pain: %s", *cleanedBiggestPain))
			return
		}
	}

	if req.ContentTopics != nil {
		if len(*req.ContentTopics) > 10 {
			writeError(w, http.StatusBadRequest, "content_topics exceeds maximum of 10 items")
			return
		}
		for _, topic := range *req.ContentTopics {
			if len(topic) > 40 {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("topic %q exceeds maximum length of 40 characters", topic))
				return
			}
			if strings.TrimSpace(topic) == "" {
				writeError(w, http.StatusBadRequest, "content topic cannot be empty")
				return
			}
		}
	}

	if req.InsightPriorities != nil {
		if len(*req.InsightPriorities) > 3 {
			writeError(w, http.StatusBadRequest, "insight_priorities exceeds maximum of 3 items")
			return
		}
		for _, priority := range *req.InsightPriorities {
			if !validInsightPriorities[priority] {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid insight_priority: %s", priority))
				return
			}
		}
	}

	existing, err := h.Queries.GetUserProfile(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load existing profile")
		return
	}
	if existing == nil {
		existing = &db.UserProfile{UserID: userID}
	}

	if req.DisplayName != nil {
		existing.DisplayName = cleanOptionalString(req.DisplayName)
	}
	if req.UserType != nil {
		existing.UserType = cleanedUserType
	}
	if req.PrimaryGoal != nil {
		existing.PrimaryGoal = cleanedPrimaryGoal
	}
	if req.PrimaryPlatform != nil {
		existing.PrimaryPlatform = cleanedPrimaryPlatform
	}
	if req.ContentTopics != nil {
		existing.ContentTopics = *req.ContentTopics
	}
	if req.AudienceSizeBucket != nil {
		existing.AudienceSizeBucket = cleanedAudienceSizeBucket
	}
	if req.WeeklyCommentVolumeBucket != nil {
		existing.WeeklyCommentVolumeBucket = cleanedWeeklyCommentVolumeBucket
	}
	if req.SuccessMetric != nil {
		existing.SuccessMetric = cleanedSuccessMetric
	}
	if req.Role != nil {
		existing.Role = cleanedRole
	}
	if req.TeamSizeBucket != nil {
		existing.TeamSizeBucket = cleanedTeamSizeBucket
	}
	if req.PublishingFrequencyBucket != nil {
		existing.PublishingFrequencyBucket = cleanedPublishingFrequencyBucket
	}
	if req.InsightPriorities != nil {
		existing.InsightPriorities = *req.InsightPriorities
	}
	if req.CurrentWorkflow != nil {
		existing.CurrentWorkflow = cleanedCurrentWorkflow
	}
	if req.BiggestPain != nil {
		existing.BiggestPain = cleanedBiggestPain
	}

	saved, err := h.Queries.UpsertUserProfile(r.Context(), existing)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save profile")
		return
	}

	if saved.ContentTopics == nil {
		saved.ContentTopics = []string{}
	}
	if saved.InsightPriorities == nil {
		saved.InsightPriorities = []string{}
	}

	writeJSON(w, http.StatusOK, saved)
}
