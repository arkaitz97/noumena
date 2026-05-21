# Actionable Insights Extraction Specification

## Purpose

Defines how the LLM extracts structured actionable insights from comments during batch classification. Each comment may yield zero or more insights with a type, description, and confidence score.

## Requirements

### Requirement: Insight Extraction During Classification
The system SHALL extract actionable insights from each comment as part of the batch classification process.

#### Scenario: Comment Yields Actionable Insights
- **GIVEN** a comment containing a suggestion for the creator
- **WHEN** the LLM processes the batch
- **THEN** the system SHALL return one or more insight objects, each with `type` (one of: video_idea, editing_tip, feature_request, collaboration_ask, improvement_suggestion, content_format_request, other), `description` (the extracted suggestion, max 200 characters), and `confidence` (number in [0.0, 1.0])

#### Scenario: Comment Yields No Actionable Insights
- **GIVEN** a comment that is purely praise, spam, or contains no actionable suggestion
- **WHEN** the LLM processes the batch
- **THEN** the system SHALL return an empty actionable_insights array for that comment

#### Scenario: Maximum Insights Per Comment
- **GIVEN** a comment with many suggestions
- **WHEN** the LLM extracts insights
- **THEN** the system SHALL return at most 5 insights for a single comment

#### Scenario: Insight Confidence Threshold
- **GIVEN** an extracted insight with confidence below the configured threshold (default 0.7)
- **WHEN** the processor applies the confidence threshold
- **THEN** the system SHALL exclude that insight from the persisted actionable_insights array

### Requirement: Insight Type Validation
The system SHALL validate that each insight type is one of the allowed set.

#### Scenario: Valid Insight Type
- **GIVEN** an insight with type "video_idea"
- **WHEN** the processor validates the classification response
- **THEN** the system SHALL accept the insight

#### Scenario: Invalid Insight Type
- **GIVEN** an insight with type not in the allowed set
- **WHEN** the processor validates the classification response
- **THEN** the system SHALL reject the entire batch and log a parse error

### Requirement: Insight Description Length Limit
The system SHALL enforce a maximum description length of 200 characters per insight.

#### Scenario: Description Within Limit
- **GIVEN** an insight description of 150 characters
- **WHEN** the processor validates the response
- **THEN** the system SHALL accept the insight unchanged

#### Scenario: Description Exceeds Limit
- **GIVEN** an insight description exceeding 200 characters
- **WHEN** the processor validates the response
- **THEN** the system SHALL truncate the description to 200 characters and append "..."

### Requirement: Actionable Insights Persistence
The system SHALL persist actionable_insights as a JSONB column on the comments table.

#### Scenario: Persist Insights with Classification
- **GIVEN** a batch of classified comments with actionable_insights
- **WHEN** the processor persists results
- **THEN** the system SHALL update the `actionable_insights` JSONB column alongside category, topic, and confidence in the same transaction

#### Scenario: Empty Insights Persistence
- **GIVEN** a comment with an empty actionable_insights array
- **WHEN** the processor persists results
- **THEN** the system SHALL store an empty JSON array `[]` in the `actionable_insights` column
