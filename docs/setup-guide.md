# Setup Guide

Manual steps required to configure Noumena for development or production.

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Environment File](#2-environment-file)
3. [Google Cloud Console (SSO + YouTube)](#3-google-cloud-console-sso--youtube)
4. [Stripe (Billing)](#4-stripe-billing)
5. [SMTP (Transactional Email)](#5-smtp-transactional-email)
6. [LLM Provider (Comment Processing)](#6-llm-provider-comment-processing)
7. [Secrets Generation](#7-secrets-generation)
8. [Database](#8-database)
9. [Production Checklist](#9-production-checklist)

---

## 1. Prerequisites

- Go 1.25+
- Node.js 20+
- A domain name (production only)
- HTTPS certificate / reverse proxy (production only; Coolify handles this)

---

## 2. Environment File

```sh
cp .env.example .env
```

Edit `.env` with your values. Every variable is documented inline. The
sections below explain where to obtain each credential.

---

## 3. Google Cloud Console (SSO + YouTube)

This is **required** for both sign-in (Google SSO) and YouTube channel
tracking. You need **two separate OAuth 2.0 Client IDs**.

### 3.1 Create a GCP project

1. Go to https://console.cloud.google.com/projectcreate
2. Name it (e.g. "Noumena") and click **Create**.

### 3.2 Enable the YouTube Data API v3

1. Go to https://console.cloud.google.com/apis/library/youtube.googleapis.com
2. Select your project and click **Enable**.

### 3.3 Configure the OAuth consent screen

1. Go to https://console.cloud.google.com/apis/credentials/consent
2. Choose **External** user type (or Internal if using Google Workspace).
3. Fill in the required fields (app name, support email, developer contact).
4. **Scopes:** Add `../auth/youtube.readonly` and `../auth/userinfo.email` and `../auth/userinfo.profile`.
5. **Test users:** Add your email if in testing mode.
6. Publish the app when ready for production.

### 3.4 Create Client A — Google SSO (sign-in)

1. Go to https://console.cloud.google.com/apis/credentials
2. Click **+ Create Credentials** → **OAuth client ID**.
3. Application type: **Web application**.
4. Name: `Noumena SSO`.
5. **Authorized JavaScript origins:**
   - `http://localhost:5173` (Vite dev server)
   - `https://yourdomain.com` (production)
6. **Authorized redirect URIs:**
   - `http://localhost:8080/auth/google/callback` (backend dev)
   - `http://localhost:5173/auth/google/callback` (SPA popup dev)
   - `https://yourdomain.com/api/auth/google/callback` (production redirect)
   - `https://yourdomain.com/auth/google/callback` (production SPA popup)
7. Click **Create**. Copy the **Client ID** and **Client Secret**.
8. Set in `.env`:
   ```
   GOOGLE_SSO_CLIENT_ID=<client-id>
   GOOGLE_SSO_CLIENT_SECRET=<client-secret>
   GOOGLE_SSO_REDIRECT_URL=http://localhost:8080/auth/google/callback
   ```

### 3.5 Create Client B — YouTube OAuth (channel tracking)

1. Go to https://console.cloud.google.com/apis/credentials
2. Click **+ Create Credentials** → **OAuth client ID**.
3. Application type: **Web application**.
4. Name: `Noumena YouTube`.
5. **Authorized redirect URIs:**
   - `http://localhost:8080/oauth/youtube/callback` (dev)
   - `https://yourdomain.com/oauth/youtube/callback` (production)
6. Click **Create**. Copy the **Client ID** and **Client Secret**.
7. Set in `.env`:
   ```
   YOUTUBE_OAUTH_CLIENT_ID=<client-id>
   YOUTUBE_OAUTH_CLIENT_SECRET=<client-secret>
   YOUTUBE_OAUTH_REDIRECT_URL=http://localhost:8080/oauth/youtube/callback
   ```

 > **Important:** These two clients must be separate. Using the same client
> for SSO and YouTube will break the OAuth state nonce validation.
>
> **Coolify note:** The redirect URIs above assume the frontend and API
> share the same domain (Coolify proxy routes `/auth/*` and `/oauth/*`
> from the frontend domain to the backend). For the custom domain
> `https://noumena.example.com`, use:
> - `https://noumena.example.com/auth/google/callback` (SSO)
> - `https://noumena.example.com/oauth/youtube/callback` (YouTube)

---

## 4. Stripe (Billing)

Only needed if `BILLING_ENABLED=true`. Skip if you don't need payments.

### 4.1 Create a Stripe account

1. Go to https://dashboard.stripe.com/register
2. Complete onboarding. No need to go live immediately — use test mode.

### 4.2 Get API keys

1. Go to https://dashboard.stripe.com/apikeys
2. Copy the **Publishable key** (starts with `pk_test_`) and **Secret key**
   (starts with `sk_test_`).
3. Set in `.env`:
   ```
   STRIPE_SECRET_KEY=sk_test_<rest>
   ```

The publishable key goes in the frontend (not in `.env` — the backend
doesn't use it).

### 4.3 Create products and prices

Create **3 products**, each with **2 prices** (monthly + annual) = 6 prices total.

1. Go to https://dashboard.stripe.com/products
2. Click **+ Add product**.
3. For each product:
   - **Name:** e.g. "Creator", "Studio", "Agency"
   - **Description:** Brief plan description
   - **Pricing model:** Standard pricing
   - For monthly: create a price (e.g. $29/month)
   - For annual: create a second price (e.g. $290/year)
4. After creation, go to the **Pricing** tab of each product.
5. Copy each **Price ID** (starts with `price_`). You need 6 total.
6. Set in `.env`:
   ```
   STRIPE_PRICE_CREATOR_MONTHLY=price_<id>
   STRIPE_PRICE_CREATOR_ANNUAL=price_<id>
   STRIPE_PRICE_STUDIO_MONTHLY=price_<id>
   STRIPE_PRICE_STUDIO_ANNUAL=price_<id>
   STRIPE_PRICE_AGENCY_MONTHLY=price_<id>
   STRIPE_PRICE_AGENCY_ANNUAL=price_<id>
   ```

### 4.4 Configure the webhook endpoint

The backend listens for Stripe events at `POST /webhooks/stripe`.

1. Go to https://dashboard.stripe.com/webhooks
2. Click **+ Add endpoint**.
3. **Endpoint URL:** `https://yourdomain.com/webhooks/stripe` (use
   `https://yourdomain.com/api/webhooks/stripe` if behind a path prefix).
4. **Events to listen for:**
   - `checkout.session.completed`
   - `invoice.payment_failed`
   - `customer.subscription.deleted`
   - `customer.subscription.updated`
5. Click **+ Add events** → select each → **Add events**.
6. Click **Add endpoint**.
7. Under **Signing secret**, click **Reveal**.
8. Copy the secret (starts with `whsec_`).
9. Set in `.env`:
   ```
   STRIPE_WEBHOOK_SECRET=whsec_<rest>
   ```

> For local development, use the Stripe CLI to forward events:
> ```sh
> stripe listen --forward-to localhost:8080/webhooks/stripe
> ```
> It prints a `whsec_` secret — use that for `STRIPE_WEBHOOK_SECRET`
> during local testing.

### 4.5 Set checkout URLs

```
BILLING_CHECKOUT_SUCCESS_URL=http://localhost:3000/billing/success
BILLING_CHECKOUT_CANCEL_URL=http://localhost:3000/billing/cancel
```

Change to your production frontend URLs when deploying.

### 4.6 Enable billing

```
BILLING_ENABLED=true
```

---

## 5. SMTP (Transactional Email)

Only needed if `EMAIL_ENABLED=true`. Skip to use no-op logging.

### 5.1 Choose an SMTP provider

Any provider works. Common choices:

| Provider | Docs | Notes |
|---|---|---|
| **SendGrid** | https://sendgrid.com/docs/ | 100 emails/day free |
| **AWS SES** | https://aws.amazon.com/ses/ | Requires domain verification |
| **Postmark** | https://postmarkapp.com/ | High deliverability |
| **Mailgun** | https://www.mailgun.com/ | Flexible API + SMTP |
| **Mailjet** | https://www.mailjet.com/ | Free tier: 200/day |

### 5.2 Get SMTP credentials

Each provider gives you:
- SMTP hostname (e.g. `smtp.sendgrid.net`)
- Port (587 for STARTTLS, 465 for implicit TLS)
- Username (often an API key)
- Password (or API secret)

### 5.3 Configure DNS records (production only)

For the domain you use in `EMAIL_FROM_ADDRESS`, add these DNS records:

**SPF record** (TXT record):
```
v=spf1 include:<provider-spf-include> ~all
```

**DKIM record** (TXT record):
Provider generates this. Usually a selector record like:
```
<selector>._domainkey.<domain>  TXT  "v=DKIM1; k=rsa; p=<public-key>"
```

**DMARC record** (TXT record for `_dmarc.<domain>`):
```
v=DMARC1; p=quarantine; rua=mailto:<report-email>
```

Verify DNS propagation:
```sh
dig TXT _dmarc.<yourdomain.com>
dig TXT <selector>._domainkey.<yourdomain.com>
```

### 5.4 Set env vars

```
EMAIL_ENABLED=true
EMAIL_FROM_ADDRESS=noreply@yourdomain.com
EMAIL_FROM_NAME=Noumena
SMTP_HOST=smtp.yourprovider.com
SMTP_PORT=587
SMTP_USERNAME=<username-or-api-key>
SMTP_PASSWORD=<password-or-api-secret>
APP_SUPPORT_EMAIL=support@yourdomain.com
```

### 5.5 Legacy Resend (alternative, optional)

If you prefer Resend over SMTP and `EMAIL_ENABLED=false`:

1. Go to https://resend.com/api-keys
2. Create an API key.
3. Set in `.env`:
   ```
   RESEND_API_KEY=re_<rest>
   ```

---

## 6. LLM Provider (Comment Processing)

Only needed if `LLM_PROCESSOR_ENABLED=true`.

### 6.1 Choose a provider

Three options, configured via `LLM_PRIMARY_PROVIDER`:

| Provider | `LLM_PRIMARY_PROVIDER` | Where to get key |
|---|---|---|
| **OpenCode** | `opencode` (default) | Organization-specific |
| **OpenAI** | `openai` | https://platform.openai.com/api-keys |
| **Anthropic** | `anthropic` | https://console.anthropic.com/settings/keys |

### 6.2 Primary provider

Set the API key for your chosen primary:

**OpenCode:**
```
OPENCODE_API_KEY=<key>
OPENCODE_BASE_URL=https://api.opencode.local/v1
OPENCODE_MODEL=opencode/default
```

**OpenAI:**
```
OPENAI_API_KEY=sk-<rest>
OPENAI_MODEL=gpt-4o
```

**Anthropic:**
```
ANTHROPIC_API_KEY=sk-ant-<rest>
ANTHROPIC_MODEL=claude-3-5-sonnet-latest
```

### 6.3 Fallback provider (optional)

When `LLM_FAILOVER_ERROR_THRESHOLD` errors occur within
`LLM_FAILOVER_WINDOW_SECONDS`, the processor switches to
`LLM_FALLBACK_PROVIDER`. Set its credentials too.

### 6.4 Enable

```
LLM_PROCESSOR_ENABLED=true
```

---

## 7. Secrets Generation

Several `.env` values need cryptographically random strings. Generate them
on the command line:

```sh
# JWT_SECRET (32+ bytes, recommend 64)
openssl rand -base64 48

# EXPORT_SIGNED_URL_SECRET (32+ bytes)
openssl rand -base64 48

# ENCRYPTION_KEY (raw key bytes; must be 16, 24, or 32 bytes)
openssl rand -base64 24
```

Update your `.env`:

```
JWT_SECRET=<base64-output>
EXPORT_SIGNED_URL_SECRET=<base64-output>
ENCRYPTION_KEY=<base64-output>
```

> **`ENCRYPTION_KEY` warning:** This key encrypts OAuth tokens at rest in
> the database. It is **mandatory** — the server refuses to start without
> it and there is no fallback key. If you lose it, all connected YouTube
> channels will break and users must re-authenticate. Store it securely
> (e.g. a password manager, Vault, or AWS Secrets Manager).

### 7.1 Rotating `ENCRYPTION_KEY`

Ciphertext is versioned (`v1:<keyID>:...`), so rotation does not require
re-encrypting the database. Procedure:

1. Generate a new key: `openssl rand -base64 24`.
2. Set `ENCRYPTION_KEY=<new-key>` and `ENCRYPTION_KEY_PREVIOUS=<old-key>`
   in the environment.
3. Restart. Existing rows still decrypt (old key matches the previous
   slot); new writes use the new key. Tokens are re-encrypted lazily on
   the next refresh/update.
4. After the new key has covered all active tokens (or after a grace
   period), remove `ENCRYPTION_KEY_PREVIOUS` and restart. Ciphertext
   written with the discarded old key then fails to decrypt with a clear
   error — keep the previous key around until you are sure it is unused.

### 7.2 Trusted proxy header (`TRUST_PROXY_HEADER`)

Rate limiting and audit logs derive the client IP from a source the client
cannot spoof:

- **No proxy** (default): `TRUST_PROXY_HEADER` unset — `RemoteAddr` is
  used; client-supplied `X-Forwarded-For` / `X-Real-IP` are **ignored**.
- **Behind a reverse proxy** (e.g. Coolify/Caddy/Traefik): set
  `TRUST_PROXY_HEADER` to the single header your proxy sets with the real
  client IP (usually `X-Real-IP`, or `X-Forwarded-For` if the proxy
  overwrites it). The value must parse as an IP; the right-most entry of a
  comma-separated chain (the peer the proxy observed) is used.

Configure the proxy to set that header itself and strip any client-supplied
value, otherwise clients can pick their own rate-limit bucket.

---

## 8. Database

Noumena uses SQLite. The schema is embedded in the binary and migrations
run automatically on startup.

1. The database file is created at `DATABASE_PATH` (default `./noumena.db`).
2. Set `STRICT_CONFIG=1` during development to catch missing required vars:
   ```
   STRICT_CONFIG=1
   ```
3. For production, ensure the directory is writable by the API process and
   has enough disk space (comment storage grows over time).
4. **Coolify:** Set `DATABASE_PATH=/data/noumena.db` and mount a persistent
   volume at `/data`. Without it the database is wiped on every deploy.

---

## 9. Production Checklist

### 9.1 Preferred: Deploy via Coolify + CI/CD pipeline

Images are built by Woodpecker CI and pushed to
`registry.arkaitzmolinuevo.com`. See [`deploy-coolify.md`](deploy-coolify.md)
for full setup.

| Environment | Image tag | Trigger |
|---|---|---|
| **Staging** | `:staging` | Auto-deployed on every `git push origin main` |
| **Production** | `:vX.Y.Z` | Deploy a specific version by pushing a tag |

The pipeline:
1. Push to `main` → Woodpecker builds → pushes `:staging` → Coolify staging redeploys
2. `git tag v1.2.3 && git push origin v1.2.3` → Woodpecker builds → pushes
   `:v1.2.3` + `:production` → Coolify production redeploys (auto or manual)

### 9.2 Woodpecker secrets required

Set these in **Woodpecker UI → Settings → Secrets**:

| Secret | Purpose |
|---|---|
| `REGISTRY_USERNAME` | Docker registry login for `registry.arkaitzmolinuevo.com` |
| `REGISTRY_PASSWORD` | Docker registry password or token |
| `COOLIFY_STAGING_WEBHOOK` | Deploy webhook from Coolify staging resource |
| `COOLIFY_PRODUCTION_WEBHOOK` | Deploy webhook from Coolify production resource |

### 9.3 Set `APP_BASE_URL`

```
APP_BASE_URL=https://noumena.example.com
```

Used in:
- Email verification links
- Password reset links
- CORS allowed origins

### 9.4 Verify feature flags

Review each flag and set to `true` for the features you want:

| Flag | Purpose |
|---|---|
| `BILLING_ENABLED` | Stripe billing |
| `EMAIL_ENABLED` | Transactional email |
| `LLM_PROCESSOR_ENABLED` | AI comment classification |
| `INSIGHTS_API_ENABLED` | Insights/analytics endpoints |
| `LIFECYCLE_WORKERS_ENABLED` | Account deletion + GDPR exports |
| `ACCOUNT_LIFECYCLE_API_ENABLED` | User-facing deletion/export APIs |
| `EXPORT_WORKER_ENABLED` | Async CSV/JSON export worker |
| `DIGEST_WORKER_ENABLED` | Weekly digest generation |
| `RATE_LIMIT_ENABLED` | Request rate limiting |
| `OTEL_ENABLED` | OpenTelemetry observability |
| `INGESTION_WORKER_ENABLED` | Comment ingestion (default: true) |

### 9.5 Storage directories

In Coolify, mount a persistent volume at `/data`. Inside the container,
set env vars to point there:

```
DATABASE_PATH=/data/noumena.db
EXPORT_DIR=/data/exports
EXPORT_STORAGE_DIR=/data/data-exports
```

The Dockerfile already creates an `app` user — no manual `mkdir` needed.

### 9.6 Monitor disk space

Comment storage and export files grow over time. Set up monitoring on
`EXPORT_DIR` and `EXPORT_STORAGE_DIR`. See
[`docs/ops/runbooks/export-disk-monitoring.md`](ops/runbooks/export-disk-monitoring.md).

### 9.7 Verify everything

1. `GET /health` returns 200.
2. Sign in with Google SSO works end-to-end.
3. Adding a YouTube channel completes the OAuth flow.
4. Comments appear after the ingestion poll interval.
5. Stripe Checkout creates a subscription (if billing enabled).
6. Email verification arrives (if email enabled).
7. Account deletion and GDPR export work (if lifecycle enabled).
