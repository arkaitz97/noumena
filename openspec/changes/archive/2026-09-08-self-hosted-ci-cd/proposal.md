## Why

GitHub Actions was the initial CI choice but we want a self-hosted pipeline to keep build infrastructure under our control, avoid runner-minute limits, and reduce external dependencies. The pipeline builds Docker images on push to `main` or version tags and pushes them to our self-hosted registry at `registry.arkaitzmolinuevo.com`.

## What Changes

- Remove `.github/workflows/docker-build.yml` (GitHub Actions workflow)
- Deploy **Woodpecker CI** (self-hosted) on the Coolify server
- Create `.woodpecker.yml` pipeline config that:
  - Builds + pushes `:staging` and `:main-<sha>` on push to `main`
  - Builds + pushes `:<version>` and `:production` on push to `v*` tags
  - Calls Coolify webhooks to trigger staging/production deployment
- Document the Woodpecker setup and deployment in the existing guides

## Capabilities

### New Capabilities
- `self-hosted-ci`: Build pipeline running on our own infrastructure, triggered by GitHub webhooks

### Modified Capabilities

None — CI/CD is a new concern, no existing spec changes.

## Impact

- Go backend: none
- Vue frontend: none
- Docker: the existing `Dockerfile` and registry are reused unchanged
- Coolify: deployment webhooks remain the same; Coolify pulls from the same registry
- Deleted: `.github/workflows/docker-build.yml`
- New infra: Woodpecker CI server + agent (Docker containers on the Coolify VM)
