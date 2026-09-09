## MODIFIED Requirements

### Requirement: Client identity from a trusted source

The system SHALL derive the client IP used for rate limiting from a source the client cannot spoof.

#### Scenario: Spoofed forwarded header
- **GIVEN** a client sending a forged `X-Forwarded-For` header
- **WHEN** the request is rate limited
- **THEN** the spoofed value is ignored and the trusted source (proxy-configured header or remote address) is used

### Requirement: Rate limiting after authentication

The system SHALL apply per-user rate limits to authenticated requests rather than treating them as anonymous.

#### Scenario: Authenticated request exceeds user limit
- **GIVEN** an authenticated session
- **WHEN** the user exceeds their per-user limit
- **THEN** the request is rejected with 429

#### Scenario: Anonymous request
- **GIVEN** no authenticated session
- **WHEN** the request is rate limited
- **THEN** it is keyed by the trusted client IP
