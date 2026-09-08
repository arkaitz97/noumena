# YouTube OAuth Setup

Google Cloud Console project setup and required scopes for YouTube channel
tracking.

## Symptoms

- Channel connect fails at the OAuth consent screen.
- `invalid_client` / `redirect_uri_mismatch` errors.

## Verification

- `YOUTUBE_OAUTH_CLIENT_ID` / `YOUTUBE_OAUTH_CLIENT_SECRET` set, with
  `YOUTUBE_OAUTH_REDIRECT_URL` registered exactly as an authorized redirect URI.

## Setup

1. Google Cloud Console → create/select a project.
2. Enable the **YouTube Data API v3**.
3. OAuth consent screen → add scopes:
   - `https://www.googleapis.com/auth/youtube.readonly` (comment ingestion).
   - `https://www.googleapis.com/auth/youtube.force-ssl` (write where needed).
4. Credentials → OAuth 2.0 Client ID (Web application) → add the authorized
   redirect URI from `YOUTUBE_OAUTH_REDIRECT_URL`
   (e.g. `https://api.noumena/oauth/youtube/callback`).
5. Record client ID/secret into `.env` / secret manager.

## Recovery

- If tokens are revoked: users reconnect via the channel connect flow; the
  worker forces a token refresh before every YouTube API call.
- **Note:** Google SSO (sign-in) uses a *separate* OAuth client
  (`GOOGLE_SSO_*`) — do not reuse the YouTube client.

## Postmortem

- Confirm the redirect URI matches byte-for-byte (trailing slashes matter).
