## ADDED Requirements

### Requirement: Campaign funnel summary

The system SHALL expose an administrator-only summary of the campaign cohort computed over the cohort view, including: total attributed users, count with OAuth completed, outreach-to-OAuth completion rate, count and rate of Activated Channel Connections (ACC), median time-to-first-insight, share of users viewing at least three distinct insights, and high-intent action rate. The summary SHALL accept an optional campaign start date filter.

#### Scenario: Summary reports ACC

- **WHEN** an administrator requests the campaign summary
- **THEN** the response includes the ACC count and the ACC rate over attributed users

#### Scenario: Summary reports time-to-first-insight

- **WHEN** an administrator requests the campaign summary
- **THEN** the response includes the median seconds from OAuth completion to first insight view for users who have both

#### Scenario: Summary respects the start date filter

- **WHEN** an administrator requests the summary with a start date
- **THEN** only users whose signup is on or after that date are counted

#### Scenario: Empty cohort is handled

- **WHEN** no attributed users match the filter
- **THEN** the response returns zero counts and null rates without error

### Requirement: Campaign decision bucket

The system SHALL classify the campaign outcome into GREEN, YELLOW, or RED using the framework thresholds on ACC, and SHALL return the bucket and the diagnostic signals that determined it.

#### Scenario: Green outcome

- **WHEN** ACC is at least 40
- **THEN** the summary reports the GREEN bucket

#### Scenario: Yellow outcome

- **WHEN** ACC is between 15 and 39 inclusive
- **THEN** the summary reports the YELLOW bucket

#### Scenario: Red outcome

- **WHEN** ACC is below 15
- **THEN** the summary reports the RED bucket

### Requirement: Campaign attribution breakouts

The system SHALL break the cohort down by campaign attribution dimensions so campaign angles and sources can be compared. Breakouts SHALL include at least the campaign angle (`utm_campaign`) and the self-reported source, with ACC count and rate per bucket.

#### Scenario: Breakout by campaign angle

- **WHEN** an administrator requests the campaign summary
- **THEN** the response includes per-`utm_campaign` ACC counts and rates

#### Scenario: Breakout by self-reported source

- **WHEN** an administrator requests the campaign summary
- **THEN** the response includes per-SRA-source ACC counts and rates

### Requirement: Campaign cohort listing

The system SHALL expose an administrator-only, paginated cohort listing with one row per attributed user containing the fields from the cohort view: user id, channel handle, touchpoint date, attribution parameters, self-reported source, OAuth completion time, first insight time, high-intent flag, campaign angle, and the derived ACC flag.

#### Scenario: Cohort rows are paginated

- **WHEN** an administrator requests the cohort listing without a page size
- **THEN** the response returns a bounded default number of rows and a cursor for the next page

#### Scenario: Cohort rows include derived flags

- **WHEN** an administrator requests the cohort listing
- **THEN** each row includes the ACC flag and high-intent flag

### Requirement: Campaign CSV export

The system SHALL expose an administrator-only CSV export of the campaign cohort using the same columns as the cohort listing.

#### Scenario: CSV export downloads

- **WHEN** an administrator requests the CSV export
- **THEN** the response is a CSV file with a header row and the cohort rows

#### Scenario: CSV export requires admin

- **WHEN** a non-administrator requests the CSV export
- **THEN** the system responds 403 and returns no file
