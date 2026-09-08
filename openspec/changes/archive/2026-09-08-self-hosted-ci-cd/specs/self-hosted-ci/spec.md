## ADDED Requirements

### Requirement: Pipeline triggers on push to main
The CI system SHALL start a new build whenever code is pushed to the `main` branch.

#### Scenario: Push to main triggers build
- **WHEN** a push to `main` is received via GitHub webhook
- **THEN** the CI system starts a new pipeline run

#### Scenario: Push to non-main branch does not trigger
- **WHEN** a push to a non-main branch is received
- **THEN** the CI system ignores the event

### Requirement: Pipeline triggers on version tag
The CI system SHALL start a new build whenever a tag matching `v*` is pushed.

#### Scenario: Push version tag triggers build
- **WHEN** a `v*` tag (e.g. `v1.2.3`) is pushed
- **THEN** the CI system starts a new pipeline run

### Requirement: Docker image is built from source
The pipeline SHALL build a Docker image using the Dockerfile at the repository root.

#### Scenario: Image builds successfully
- **WHEN** the pipeline runs
- **THEN** `docker build` executes using the repo's Dockerfile
- **THEN** the build produces a runnable Docker image

#### Scenario: Build failure aborts pipeline
- **WHEN** `docker build` exits with a non-zero code
- **THEN** the pipeline stops and reports failure

### Requirement: Image is pushed to self-hosted registry
The pipeline SHALL push built images to `registry.arkaitzmolinuevo.com/noumena`.

#### Scenario: Image pushed with correct tags on main
- **WHEN** the build completes on a `main` branch push
- **THEN** the image is tagged as `:staging` and `:main-<short-sha>`
- **THEN** both tags are pushed to the registry

#### Scenario: Image pushed with tags on version tag
- **WHEN** the build completes on a `v*` tag push
- **THEN** the image is tagged as `:<version>` (e.g. `:v1.2.3`) and `:production`
- **THEN** both tags are pushed to the registry

### Requirement: Registry credentials are not in the repo
Docker registry credentials SHALL be stored as CI secrets, never in the source code.

#### Scenario: Credentials injected from secrets
- **WHEN** the pipeline runs a `docker push` step
- **THEN** credentials are sourced from CI secrets, not the repository

### Requirement: Coolify deploys after successful build
The pipeline SHALL trigger a Coolify deployment after a successful image push.

#### Scenario: Staging webhook called on main push
- **WHEN** the image push completes on a `main` branch build
- **THEN** the pipeline HTTP POSTs to the Coolify staging webhook URL

#### Scenario: Production webhook called on tag push
- **WHEN** the image push completes on a `v*` tag build
- **THEN** the pipeline HTTP POSTs to the Coolify production webhook URL

#### Scenario: Webhook failure does not fail the pipeline
- **WHEN** the webhook call returns a non-2xx status
- **THEN** the pipeline logs a warning
- **THEN** the pipeline continues and reports success
