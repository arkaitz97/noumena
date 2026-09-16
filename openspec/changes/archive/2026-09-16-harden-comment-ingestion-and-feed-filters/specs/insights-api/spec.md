## MODIFIED Requirements

### Requirement: Comment Feed Filters

The feed API SHALL apply category, insight, intent, date, search, channel, toxicity, and pagination filters according to one documented contract, with predicates composed deterministically.

#### Scenario: Insight type appears after the first JSON element

- **GIVEN** a comment has multiple actionable insights and the requested type matches only the second or later element
- **WHEN** the feed is requested with that insight type
- **THEN** the comment SHALL be returned

#### Scenario: Ideas intent uses OR semantics

- **GIVEN** a comment has category `content_idea` but no `video_idea` insight, or has a `video_idea` insight but another category
- **WHEN** the feed is requested with `intent=ideas`
- **THEN** the comment SHALL be returned in either case
- **AND** additional channel/date/search/toxicity filters SHALL still apply

#### Scenario: Explicit empty categories

- **GIVEN** the request explicitly encodes an empty category selection
- **WHEN** the feed is queried
- **THEN** the category predicate SHALL match no comments
- **AND** it SHALL not silently fall back to the default high-signal categories

#### Scenario: Omitted categories

- **GIVEN** the request omits the categories parameter
- **WHEN** the feed is queried
- **THEN** the API SHALL apply the documented canonical default category set
- **AND** that set SHALL be `question`, `content_idea`, `collaboration_ask`, `praise`, and `constructive_criticism`

#### Scenario: Date-only end boundary

- **GIVEN** `date_to` is a date-only value
- **WHEN** the feed is queried
- **THEN** comments posted at any time on that UTC calendar day SHALL be included
- **AND** comments on the following day SHALL be excluded

#### Scenario: Literal search characters

- **GIVEN** a user searches for `%` or `_`
- **WHEN** the feed is queried
- **THEN** those characters SHALL be treated literally rather than as SQL `LIKE` wildcards

### Requirement: Intent and aggregate scope

The API SHALL expose intent predicates and SHALL apply the same complete feed scope to dashboard aggregate responses.

#### Scenario: Multi-channel aggregate scope

- **GIVEN** a user selects multiple channels and a custom date range in the feed
- **WHEN** the dashboard requests aggregates
- **THEN** every aggregate SHALL cover the same selected channels and date range

#### Scenario: All-time scope reset

- **GIVEN** a user changes a date range from a bounded range to all-time
- **WHEN** feed and aggregate requests are issued
- **THEN** neither request SHALL retain the previous bounded dates

### Requirement: Saved comments defaults

The Saved comments endpoint SHALL return all accessible starred comments by default, sorted by `starred_at` descending, without applying the ordinary high-signal category/toxicity defaults unless the caller explicitly requests them.

#### Scenario: Starred low-signal comment

- **GIVEN** a user starred a comment categorized as `other`, toxic, or spam
- **WHEN** the user opens Saved without explicit filters
- **THEN** the starred comment SHALL be present
