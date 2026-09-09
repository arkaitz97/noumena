## ADDED Requirements

### Requirement: Encryption key versioning

The system SHALL support multiple encryption keys and a documented rotation path without bulk data migration.

#### Scenario: Rotate to a new key
- **GIVEN** a current key and a previous key configured
- **WHEN** old ciphertext is decrypted
- **THEN** it decrypts with the previous key and re-encrypts with the current key on the next write

#### Scenario: Unset encryption key
- **GIVEN** no `ENCRYPTION_KEY` configured
- **WHEN** the application starts
- **THEN** startup fails with a clear error; no hardcoded fallback key is ever used
