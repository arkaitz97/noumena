## ADDED Requirements

### Requirement: Custom Prompt Per Channel
The system SHALL allow users to customize the LLM classification prompt for specific channels.

#### Scenario: Set Custom Prompt
- **GIVEN** an authenticated user who tracks a channel
- **WHEN** the user calls PATCH /api/channels/{id}/custom-prompt with `{prompt: "Focus on product feedback and feature requests."}`
- **THEN** the system SHALL validate the prompt (max 500 chars, no HTML/script tags), store it in user_channel_tracking.custom_prompt, and return 200

#### Scenario: Custom Prompt Validation - Too Long
- **GIVEN** an authenticated user
- **WHEN** the user submits a custom prompt longer than 500 characters
- **THEN** the system SHALL return 400 with "custom prompt must be 500 characters or less"

#### Scenario: Custom Prompt Validation - HTML Injection
- **GIVEN** an authenticated user
- **WHEN** the user submits a custom prompt containing HTML tags or script elements
- **THEN** the system SHALL return 400 with "custom prompt must not contain HTML or script tags"

#### Scenario: Clear Custom Prompt
- **GIVEN** a channel with a custom prompt set
- **WHEN** the user calls PATCH /api/channels/{id}/custom-prompt with `{prompt: ""}`
- **THEN** the system SHALL clear the custom_prompt and return 200

### Requirement: Thread-Aware Classification
The system SHALL include parent comment context when classifying reply comments.

#### Scenario: Classify Reply with Parent Context
- **WHEN** the processor classifies a comment with parent_comment_id != NULL
- **THEN** the system SHALL include the parent comment text (truncated to 200 chars) in the classification prompt as context

#### Scenario: Classify Top-Level Comment
- **WHEN** the processor classifies a comment with parent_comment_id = NULL
- **THEN** the system SHALL classify without parent context (existing behavior)
