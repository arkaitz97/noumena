package db

import (
	"context"
	"testing"
)

func TestUserProfile_DBOperations(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)

	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	q := NewSQLiteQuery(database)

	// Create a test user
	user, err := q.CreateUser(ctx, "creator@noumena.test", "hash123")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// 1. Get profile when none exists -> returns nil, nil
	prof, err := q.GetUserProfile(ctx, user.ID)
	if err != nil {
		t.Fatalf("get non-existent profile: %v", err)
	}
	if prof != nil {
		t.Fatalf("expected nil profile, got %+v", prof)
	}

	// 2. Insert profile
	displayName := "Jane Doe"
	userType := "creator"
	goal := "understand_audience"
	platform := "youtube"
	topics := []string{"tech", "coding"}
	audSize := "10k_50k"
	role := "owner"
	priorities := []string{"questions", "content_ideas"}

	newProf := &UserProfile{
		UserID:             user.ID,
		DisplayName:        &displayName,
		UserType:           &userType,
		PrimaryGoal:        &goal,
		PrimaryPlatform:    &platform,
		ContentTopics:      topics,
		AudienceSizeBucket: &audSize,
		Role:               &role,
		InsightPriorities:  priorities,
	}

	upserted, err := q.UpsertUserProfile(ctx, newProf)
	if err != nil {
		t.Fatalf("upsert profile: %v", err)
	}
	if upserted == nil {
		t.Fatal("upserted profile is nil")
	}
	if upserted.DisplayName == nil || *upserted.DisplayName != "Jane Doe" {
		t.Errorf("display_name = %v, want Jane Doe", upserted.DisplayName)
	}
	if upserted.UserType == nil || *upserted.UserType != "creator" {
		t.Errorf("user_type = %v, want creator", upserted.UserType)
	}
	if len(upserted.ContentTopics) != 2 || upserted.ContentTopics[0] != "tech" {
		t.Errorf("content_topics = %v, want [tech, coding]", upserted.ContentTopics)
	}

	// 3. Retrieve profile
	fetched, err := q.GetUserProfile(ctx, user.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if fetched == nil {
		t.Fatal("fetched profile is nil")
	}
	if fetched.PrimaryGoal == nil || *fetched.PrimaryGoal != "understand_audience" {
		t.Errorf("primary_goal = %v, want understand_audience", fetched.PrimaryGoal)
	}
	if len(fetched.InsightPriorities) != 2 || fetched.InsightPriorities[0] != "questions" {
		t.Errorf("insight_priorities = %v, want [questions, content_ideas]", fetched.InsightPriorities)
	}

	// 4. Update profile
	updatedGoal := "generate_content_ideas"
	fetched.PrimaryGoal = &updatedGoal
	updatedTopics := []string{"tech", "ai", "startups"}
	fetched.ContentTopics = updatedTopics

	updated, err := q.UpsertUserProfile(ctx, fetched)
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.PrimaryGoal == nil || *updated.PrimaryGoal != "generate_content_ideas" {
		t.Errorf("updated primary_goal = %v, want generate_content_ideas", updated.PrimaryGoal)
	}
	if len(updated.ContentTopics) != 3 {
		t.Errorf("updated topics len = %d, want 3", len(updated.ContentTopics))
	}
}
