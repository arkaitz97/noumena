# Deploy to Coolify

## Architecture

Noumena is deployed as a single Docker container that serves **both** the
Go API and the Vue SPA frontend. Images are built by Woodpecker CI and
pushed to a self-hosted Docker registry.

| Environment | Image tag | Deploy trigger |
|---|---|---|
| **Staging** | `:staging` | Auto-deployed on every push to `main` |
| **Production** | `:vX.Y.Z` (e.g. `:v1.2.3`) | Manual — deploy a specific version |

---

## Prerequisites

- A Coolify instance (self-hosted or cloud)
- Docker registry credentials for `registry.arkaitzmolinuevo.com`
- GitHub repo connected to Coolify
- Domains: `staging.noumena.example.com` and `noumena.example.com`
- GitHub secrets configured (see [step 1](#step-1--configure-github-secrets))

---

## Step 1 — Configure Woodpecker secrets

The pipeline needs these secrets in Woodpecker CI
(**Settings → Secrets** in the Woodpecker UI):

| Secret | Value |
|---|---|
| `REGISTRY_USERNAME` | Docker registry username |
| `REGISTRY_PASSWORD` | Docker registry password or token |
| `COOLIFY_STAGING_WEBHOOK` | Coolify deploy webhook for staging (set up in step 3) |
| `COOLIFY_PRODUCTION_WEBHOOK` | Coolify deploy webhook for production (set up in step 4) |

> Register the webhooks **before** the secrets — you'll paste the URLs
> after completing steps 3 and 4 below.

---

## Step 2 — Add the Docker registry in Coolify

1. Go to **Servers** → **Your server** → **Advanced**.
2. Under **Docker Registry**, add:
   - **Registry URL:** `https://registry.arkaitzmolinuevo.com`
   - **Username:** your registry username
   - **Password:** your registry password
3. Save. Coolify can now pull private images from this registry.

---

## Step 3 — Staging environment

### Create the staging service

1. **Services** → **+ New Service** → **Private Docker Hub/Registry**.
2. **Image:** `registry.arkaitzmolinuevo.com/noumena:staging`
3. **Name:** `noumena-staging`
4. **Ports Exposed:** `8080`
5. **Health Check:** `/health`

### Environment variables

| Variable | Value |
|---|---|
| `DATABASE_PATH` | `/data/noumena.db` |
| `JWT_SECRET` | `openssl rand -base64 48` |
| `APP_BASE_URL` | `https://staging.noumena.example.com` |
| `GOOGLE_SSO_CLIENT_ID` | from GCP |
| `GOOGLE_SSO_CLIENT_SECRET` | from GCP |
| `GOOGLE_SSO_REDIRECT_URL` | `https://staging.noumena.example.com/auth/google/callback` |
| `YOUTUBE_OAUTH_CLIENT_ID` | from GCP |
| `YOUTUBE_OAUTH_CLIENT_SECRET` | from GCP |
| `YOUTUBE_OAUTH_REDIRECT_URL` | `https://staging.noumena.example.com/oauth/youtube/callback` |
| `ENCRYPTION_KEY` | `openssl rand -base64 24` (mandatory, raw 32-byte key) |
| `EXPORT_SIGNED_URL_SECRET` | `openssl rand -base64 48` |
| `COOKIE_SECURE` | `true` |
| `STRICT_CONFIG` | `1` |

### Persistent volume

**Storage/Volumes** tab → add a volume:
- **Storage path:** `/data`
- **Mount path:** `/data`

### Domain

**Proxy** tab → **Domain:** `staging.noumena.example.com`

### Auto-deploy webhook

1. **Settings** → **Deploy** → enable **Auto Deploy**.
2. Coolify shows a **Webhook** URL.
3. Copy it → save as `COOLIFY_STAGING_WEBHOOK` in Woodpecker secrets.

### GCP OAuth URIs

Add these redirect URIs to your GCP OAuth clients:

| Client | Redirect URI |
|---|---|
| Google SSO | `https://staging.noumena.example.com/auth/google/callback` |
| YouTube OAuth | `https://staging.noumena.example.com/oauth/youtube/callback` |

---

## Step 4 — Production environment

### Create the production service

1. **Services** → **+ New Service** → **Private Docker Hub/Registry**.
2. **Image:** `registry.arkaitzmolinuevo.com/noumena:production`
3. **Name:** `noumena-production`
4. **Ports Exposed:** `8080`
5. **Health Check:** `/health`

### Environment variables

Same as staging, but with live credentials and your production domain:

| Variable | Value |
|---|---|
| `DATABASE_PATH` | `/data/noumena.db` |
| `JWT_SECRET` | `openssl rand -base64 48` (use a different secret) |
| `APP_BASE_URL` | `https://noumena.example.com` |
| `GOOGLE_SSO_REDIRECT_URL` | `https://noumena.example.com/auth/google/callback` |
| `YOUTUBE_OAUTH_REDIRECT_URL` | `https://noumena.example.com/oauth/youtube/callback` |
| `COOKIE_SECURE` | `true` |

> **Important:** Use live Stripe keys (`sk_live_*`) and a dedicated SMTP
> provider for production. Keep staging on test keys.

### Persistent volume

Same as staging: mount `/data` for the SQLite database.

### Domain

**Proxy** tab → **Domain:** `noumena.example.com`

### Auto-deploy webhook (optional)

If you want auto-deploy when a new tag is pushed:

1. **Settings** → **Deploy** → enable **Auto Deploy**.
2. Copy the webhook URL → save as `COOLIFY_PRODUCTION_WEBHOOK` in
   Woodpecker secrets.

Without the webhook, deploy manually by changing the image tag in the
service settings and clicking **Deploy**.

### GCP OAuth URIs

Add these to your GCP OAuth clients:

| Client | Redirect URI |
|---|---|
| Google SSO | `https://noumena.example.com/auth/google/callback` |
| YouTube OAuth | `https://noumena.example.com/oauth/youtube/callback` |

### Stripe webhook

**Stripe Dashboard → Developers → Webhooks** → update the endpoint:

```
https://noumena.example.com/webhooks/stripe
```

---

## Step 5 — Deploy a release to production

### Push a version tag

```sh
git tag v1.2.3
git push origin v1.2.3
```

Woodpecker CI runs `.woodpecker.yml`, which:
1. Builds the image
2. Tags it as `:v1.2.3` and `:production`
3. Pushes both tags to `registry.arkaitzmolinuevo.com`
4. Calls the production webhook (if configured)

### Manual deploy (without auto-deploy webhook)

1. In Coolify → **noumena-production** → **Settings**.
2. Change **Image** from `:production` to your specific tag, e.g.
   `registry.arkaitzmolinuevo.com/noumena:v1.2.3`.
3. Click **Deploy**.

> Rolling back: change the image tag back to the previous version and
> redeploy.

---

## CI/CD pipeline summary

```
push to main
    │
    ▼
Woodpecker CI ──build──► registry.arkaitzmolinuevo.com/noumena:staging
                             │
                             ▼
                        Coolify staging
                        (auto-deploy via webhook)


git tag vX.Y.Z
    │
    ▼
Woodpecker CI ──build──► registry.arkaitzmolinuevo.com/noumena:vX.Y.Z
                          registry.arkaitzmolinuevo.com/noumena:production
                             │
                             ▼
                        Coolify production
                        (auto or manual deploy)
```

---

## Useful commands

```sh
# Tail staging logs
coolify service logs noumena-staging

# Tail production logs
coolify service logs noumena-production

# Enter a running container
coolify service exec noumena-staging

# Manually redeploy staging with the latest image
coolify service deploy noumena-staging
```

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `ImagePullBackOff` | Registry credentials not set in Coolify | Check the Docker registry config in **Servers → Advanced**. |
| `unauthorized: access forbidden` | Invalid registry login | Verify `REGISTRY_USERNAME` and `REGISTRY_PASSWORD` in Woodpecker secrets. |
| `database is locked` | Multiple replicas sharing one SQLite file | Set `MAX_REPLICAS=1` in Coolify — SQLite does not support concurrent writers. |
| OAuth callback 404 | Redirect URI mismatch | Check the exact callback URL in GCP Console vs. your Coolify domain. |
| Frontend loads but API calls fail | Wrong `APP_BASE_URL` or proxy | Both API and frontend are on the same domain — proxy should just pass through. |
| `JWT_SECRET must be 32+ bytes` | Secret too short | Regenerate with `openssl rand -base64 48`. |
| Stripe webhook signing failure | Wrong `STRIPE_WEBHOOK_SECRET` | Reveal the signing secret in Stripe Dashboard and copy it exactly. |
| Production needs a different version | Image tag pinned to old version | Update the image tag in service settings or push a new git tag. |
