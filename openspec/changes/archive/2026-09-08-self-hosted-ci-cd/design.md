## Context

Currently, Docker images are built by GitHub Actions and pushed to `registry.arkaitzmolinuevo.com`. We want to move this to a self-hosted CI runner to avoid external infrastructure dependencies. The Coolify VM already runs Docker and has access to the registry.

Woodpecker CI is chosen because:
- Lightweight (~20MB binary or single Docker container)
- No database dependency (uses SQLite by default)
- Native Docker-in-Docker (DinD) for building images
- YAML pipeline format familiar from GitHub Actions
- Active open-source project (Apache 2.0)
- Can run on the same VM as Coolify with minimal overhead

## Goals / Non-Goals

**Goals:**
- Replace GitHub Actions with a self-hosted Woodpecker CI instance
- Build and push Docker images to `registry.arkaitzmolinuevo.com` on push to `main` and `v*` tags
- Trigger Coolify deployments via webhooks after successful builds
- Deploy Woodpecker as Docker containers on the Coolify VM

**Non-Goals:**
- Not migrating off GitHub (webhooks from GitHub still trigger pipelines)
- Not changing the Dockerfile or build process
- Not modifying Coolify's deployment mechanism
- Not running tests in CI (integration tests already covered by the existing `integration.yml` workflow)

## Decisions

### 1. Woodpecker over alternatives

| Option | Why not chosen |
|---|---|
| **Jenkins** | Heavy (JVM), complex plugin management, overkill for a single pipeline |
| **Drone CI** | Woodpecker is the active fork (Drone was acquired, development slowed) |
| **Concourse** | Steeper learning curve, different pipeline model (resource-oriented) |
| **Gitea Actions** | Requires self-hosting Gitea; we use GitHub |
| **Buildkite** | Not fully self-hosted (pipeline metadata goes through Buildkite cloud) |

### 2. Single-node deployment (server + agent on same VM)

Both Woodpecker server and agent run as Docker containers on the Coolify VM. This keeps latency low (no network hops for pushes) and avoids provisioning a second machine. If build volume grows, the agent can be scaled to a separate host.

### 3. DinD for Docker image builds

The Woodpecker agent runs a Docker socket mount (`/var/run/docker.sock`) to build images inside the pipeline. This is the standard approach for Docker-in-Docker CI.

### 4. Registry credentials via Woodpecker secrets

`REGISTRY_USERNAME` and `REGISTRY_PASSWORD` are stored as Woodpecker secrets and injected into the pipeline step. Never written to the repo.

### 5. Coolify webhook triggers via curl

Same approach as the GitHub Actions workflow: after a successful push, `curl -X POST` hits the Coolify deploy webhook URL. This is unchanged from the previous design.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| DinD requires privileged Docker access | The Woodpecker agent container needs `-v /var/run/docker.sock:/var/run/docker.sock`. Limit to CI-specific containers. |
| Woodpecker server has no auth by default | Set `WOODPECKER_OPEN=true` + configure `WOODPECKER_GITHUB_CLIENT_ID`/`WOODPECKER_GITHUB_CLIENT_SECRET` for GitHub OAuth login. |
| Single point of failure (one agent) | Acceptable for current scale. Woodpecker supports multiple agents — add a second agent on another host if needed. |
| Builds consume disk space (Docker layers) | Woodpecker automatically prunes old builds. Monitor disk usage on the VM. |
| Woodpecker outage stalls deployments | Pipeline can be triggered manually by running `docker build` + `docker push` on the VM and calling the Coolify webhook. Document this fallback. |

## Woodpecker deployment

Woodpecker is deployed as a **Docker Compose service** in Coolify via the
Coolify UI — no SSH or manual docker-compose commands needed.

### Docker Compose file

Create a new **Docker Compose** service in Coolify
(`Services → + New Service → Docker Compose`) and paste:

```yaml
services:
  woodpecker-server:
    image: woodpeckerci/woodpecker-server:latest
    ports: ["8000:8000"]
    volumes:
      - woodpecker-data:/var/lib/woodpecker/
    environment:
      - WOODPECKER_OPEN=true
      - WOODPECKER_GITHUB_CLIENT_ID=${WOODPECKER_GITHUB_CLIENT_ID}
      - WOODPECKER_GITHUB_CLIENT_SECRET=${WOODPECKER_GITHUB_CLIENT_SECRET}
      - WOODPECKER_AGENT_SECRET=${WOODPECKER_AGENT_SECRET}

  woodpecker-agent:
    image: woodpeckerci/woodpecker-agent:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - WOODPECKER_SERVER=woodpecker-server:9000
      - WOODPECKER_AGENT_SECRET=${WOODPECKER_AGENT_SECRET}

volumes:
  woodpecker-data:
```

### Environment variables

Set these in the Coolify UI under **Environment Variables** for the
Woodpecker service (never in the compose file or a `.env` file on disk):

| Variable | Value |
|---|---|
| `WOODPECKER_GITHUB_CLIENT_ID` | From GitHub OAuth App |
| `WOODPECKER_GITHUB_CLIENT_SECRET` | From GitHub OAuth App |
| `WOODPECKER_AGENT_SECRET` | `openssl rand -base64 32` |

### Deployment

Click **Deploy** in Coolify. Coolify manages the containers, volumes, and
networking. Woodpecker will be accessible via the Coolify proxy domain or
at `http://<vm-ip>:8000` depending on the Coolify proxy configuration.

## Pipeline flow

```
GitHub push (main / v* tag)
        │
        ▼
GitHub webhook ──POST──► Woodpecker server
        │
        ▼
Woodpecker agent picks up pipeline
        │
        ▼
Reads .woodpecker.yml
        │
        ▼
  ┌─────────────────────────┐
  │ Step: build & push      │
  │ DinD: docker buildx     │
  │ Tags: :staging / :v*    │
  │ Push to registry        │
  └─────────────────────────┘
        │
        ▼
  ┌─────────────────────────┐
  │ Step: deploy webhook    │
  │ curl POST to Coolify    │
  └─────────────────────────┘
        │
        ▼
Coolify pulls new image + redeploys
```
