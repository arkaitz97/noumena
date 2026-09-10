package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const userProfileColumns = `
	user_id,
	display_name,
	user_type,
	primary_goal,
	primary_platform,
	content_topics,
	audience_size_bucket,
	weekly_comment_volume_bucket,
	success_metric,
	role,
	team_size_bucket,
	publishing_frequency_bucket,
	insight_priorities,
	current_workflow,
	biggest_pain,
	created_at,
	updated_at
`

func scanUserProfile(row *sql.Row) (*UserProfile, error) {
	var (
		p                               UserProfile
		displayName                     sql.NullString
		userType, primaryGoal, platform sql.NullString
		contentTopicsJSON               sql.NullString
		audSize, commVol, successMet    sql.NullString
		role, teamSize, pubFreq         sql.NullString
		insightPrioritiesJSON           sql.NullString
		currWorkflow, biggestPain       sql.NullString
		createdAtStr, updatedAtStr      string
	)

	err := row.Scan(
		&p.UserID,
		&displayName,
		&userType,
		&primaryGoal,
		&platform,
		&contentTopicsJSON,
		&audSize,
		&commVol,
		&successMet,
		&role,
		&teamSize,
		&pubFreq,
		&insightPrioritiesJSON,
		&currWorkflow,
		&biggestPain,
		&createdAtStr,
		&updatedAtStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan user_profile: %w", err)
	}

	if displayName.Valid {
		p.DisplayName = &displayName.String
	}
	if userType.Valid {
		p.UserType = &userType.String
	}
	if primaryGoal.Valid {
		p.PrimaryGoal = &primaryGoal.String
	}
	if platform.Valid {
		p.PrimaryPlatform = &platform.String
	}
	if contentTopicsJSON.Valid && contentTopicsJSON.String != "" {
		_ = json.Unmarshal([]byte(contentTopicsJSON.String), &p.ContentTopics)
	}
	if audSize.Valid {
		p.AudienceSizeBucket = &audSize.String
	}
	if commVol.Valid {
		p.WeeklyCommentVolumeBucket = &commVol.String
	}
	if successMet.Valid {
		p.SuccessMetric = &successMet.String
	}
	if role.Valid {
		p.Role = &role.String
	}
	if teamSize.Valid {
		p.TeamSizeBucket = &teamSize.String
	}
	if pubFreq.Valid {
		p.PublishingFrequencyBucket = &pubFreq.String
	}
	if insightPrioritiesJSON.Valid && insightPrioritiesJSON.String != "" {
		_ = json.Unmarshal([]byte(insightPrioritiesJSON.String), &p.InsightPriorities)
	}
	if currWorkflow.Valid {
		p.CurrentWorkflow = &currWorkflow.String
	}
	if biggestPain.Valid {
		p.BiggestPain = &biggestPain.String
	}

	p.CreatedAt = parseTime(createdAtStr)
	p.UpdatedAt = parseTime(updatedAtStr)

	return &p, nil
}

func (q *SQLiteQuery) GetUserProfile(ctx context.Context, userID string) (*UserProfile, error) {
	query := `SELECT ` + userProfileColumns + ` FROM user_profiles WHERE user_id = ?`
	p, err := scanUserProfile(q.db.QueryRowContext(ctx, query, userID))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return p, err
}

func (q *SQLiteQuery) UpsertUserProfile(ctx context.Context, profile *UserProfile) (*UserProfile, error) {
	if profile == nil || profile.UserID == "" {
		return nil, errors.New("missing user_id for profile upsert")
	}

	var contentTopicsJSON *string
	if profile.ContentTopics != nil {
		b, err := json.Marshal(profile.ContentTopics)
		if err != nil {
			return nil, fmt.Errorf("marshal content_topics: %w", err)
		}
		str := string(b)
		contentTopicsJSON = &str
	}

	var insightPrioritiesJSON *string
	if profile.InsightPriorities != nil {
		b, err := json.Marshal(profile.InsightPriorities)
		if err != nil {
			return nil, fmt.Errorf("marshal insight_priorities: %w", err)
		}
		str := string(b)
		insightPrioritiesJSON = &str
	}

	query := `
		INSERT INTO user_profiles (
			user_id,
			display_name,
			user_type,
			primary_goal,
			primary_platform,
			content_topics,
			audience_size_bucket,
			weekly_comment_volume_bucket,
			success_metric,
			role,
			team_size_bucket,
			publishing_frequency_bucket,
			insight_priorities,
			current_workflow,
			biggest_pain,
			created_at,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		ON CONFLICT(user_id) DO UPDATE SET
			display_name = excluded.display_name,
			user_type = excluded.user_type,
			primary_goal = excluded.primary_goal,
			primary_platform = excluded.primary_platform,
			content_topics = excluded.content_topics,
			audience_size_bucket = excluded.audience_size_bucket,
			weekly_comment_volume_bucket = excluded.weekly_comment_volume_bucket,
			success_metric = excluded.success_metric,
			role = excluded.role,
			team_size_bucket = excluded.team_size_bucket,
			publishing_frequency_bucket = excluded.publishing_frequency_bucket,
			insight_priorities = excluded.insight_priorities,
			current_workflow = excluded.current_workflow,
			biggest_pain = excluded.biggest_pain,
			updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		RETURNING ` + userProfileColumns

	return scanUserProfile(q.db.QueryRowContext(ctx, query,
		profile.UserID,
		profile.DisplayName,
		profile.UserType,
		profile.PrimaryGoal,
		profile.PrimaryPlatform,
		contentTopicsJSON,
		profile.AudienceSizeBucket,
		profile.WeeklyCommentVolumeBucket,
		profile.SuccessMetric,
		profile.Role,
		profile.TeamSizeBucket,
		profile.PublishingFrequencyBucket,
		insightPrioritiesJSON,
		profile.CurrentWorkflow,
		profile.BiggestPain,
	))
}
