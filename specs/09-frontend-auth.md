# Spec: Frontend - Authentication & Onboarding

## Overview
Build the complete frontend authentication flow and first-time onboarding wizard for Noumena. This includes login, signup, email verification, password reset, Google SSO, and the channel connection onboarding flow.

## Tech Stack
- Vue 3 with Composition API
- Pinia for state management
- Vue Router 4
- Axios for API calls
- Tailwind CSS for styling
- Lucide Vue for icons

## Routes

| Route | Component | Auth Required | Description |
|-------|-----------|---------------|-------------|
| `/` | LandingView.vue | No | Marketing landing page |
| `/signup` | SignupView.vue | No | Email/password registration |
| `/login` | LoginView.vue | No | Login form |
| `/verify-email` | VerifyEmailView.vue | Partial | Post-registration verification gate |
| `/forgot-password` | ForgotPasswordView.vue | No | Password reset request |
| `/reset-password` | ResetPasswordView.vue | No (token-based) | Set new password |
| `/oauth/callback` | OAuthCallbackView.vue | No | Handle Google OAuth popup callback |
| `/onboarding` | OnboardingView.vue | Yes (verified) | First-time channel connection wizard |
| `/dashboard` | DashboardLayout.vue | Yes (verified) | Main app layout with sidebar |

## State Management (Pinia)

### `useAuthStore`
```typescript
interface AuthState {
  accessToken: string | null;
  user: User | null;
  isAuthenticated: boolean;
  isEmailVerified: boolean;
}

interface User {
  id: string;
  email: string;
  email_verified_at: string | null;
  subscription_plan: string;
}

// Actions:
- login(email, password): Promise<void>
- register(email, password): Promise<void>
- logout(): Promise<void>
- logoutAll(): Promise<void>
- refreshAccessToken(): Promise<boolean> // returns true if refreshed
- verifyEmail(token): Promise<void>
- resendVerification(): Promise<void>
- forgotPassword(email): Promise<void>
- resetPassword(token, password): Promise<void>
- fetchUser(): Promise<void>
```

### Token Management
- Access token stored in Pinia state only (memory)
- Refresh token stored in `HttpOnly`, `Secure`, `SameSite=Strict` cookie (handled by browser)
- Axios interceptor:
  - Attach `Authorization: Bearer ${accessToken}` to all requests
  - On 401: call `/auth/refresh`, retry original request
  - On refresh failure: clear state, redirect to `/login`
  - On 403 with code `email_not_verified`: redirect to `/verify-email`

## Views

### LandingView.vue
- Hero section with product value prop
- CTA buttons: "Get Started" → `/signup`, "Sign In" → `/login`

### SignupView.vue
- Form: email, password, confirm password
- Real-time validation: password strength indicator (min 8, 1 uppercase, 1 number)
- Google "Continue with Google" button
- Link to `/login`
- On success: show "Check your inbox" message, redirect to `/verify-email`

### LoginView.vue
- Form: email, password
- "Remember me" checkbox (extends refresh token expiry)
- Links: "Forgot password?", "Create account"
- Google SSO button
- On success: redirect to `/dashboard` (or `/onboarding` if no channels)

### VerifyEmailView.vue
- Shows "Check your inbox" message with resend button
- Can also accept `?token=` query param to auto-verify on mount
- On verification success: redirect to `/onboarding`

### ForgotPasswordView.vue
- Email input only
- Generic success message regardless of email existence (security)
- Rate limit UI feedback ("Please wait X minutes before requesting again")

### ResetPasswordView.vue
- Accepts `?token=` query param
- New password + confirm password
- Validation same as signup
- On success: redirect to `/login` with success toast

### OAuthCallbackView.vue
- Mounted in popup window
- Extracts `code` and `state` from URL query params
- Posts message to parent window with code
- Parent window then calls `POST /api/channels/exchange/youtube`

### OnboardingView.vue
- Step 1: Welcome message, explain value
- Step 2: Connect YouTube Channel
  - "Connect YouTube" button
  - Opens OAuth popup (see flow below)
  - Shows loading state while exchanging code
  - On success: shows connected channel info
- Step 3: Backfill status
  - "Syncing your channel... this may take up to 24 hours"
  - Progress indicator (can poll `/api/channels/{id}/sync-status`)
  - When backfill ≥ 10%: show "Continue to Dashboard" button
- Step 4: Brief product tour (optional, can be skip)

### DashboardLayout.vue
- Persistent sidebar with navigation:
  - Insights Feed
  - Saved Comments
  - Settings (channels, billing, account)
- Top bar with user menu (logout, logout all devices, settings)
- Main content area via `<router-view>`

## Google SSO Flow (SPA)

1. User clicks "Continue with Google"
2. Frontend calls `GET /auth/google` → receives auth URL
3. Opens `window.open(authUrl, 'google-oauth', 'width=500,height=600')`
4. Popup completes OAuth and redirects to `/oauth/callback?code=...`
5. OAuthCallbackView extracts code, calls `window.opener.postMessage({ type: 'oauth-code', code, provider: 'google' }, '*')`
6. Parent window (SignupView/LoginView) listens for message:
   - If registering: call `POST /auth/google/exchange` (endpoint TBD or reuse channel exchange pattern)
   - If logging in: call `POST /auth/google/login` (endpoint TBD)
7. Popup closes automatically

**Note**: The PRD describes Google SSO for auth, but the backend currently only has channel OAuth. A decision is needed: should auth Google SSO use the same OAuth config or separate credentials? Recommend separate Google OAuth app for auth vs channel access, or use the same app with multiple scopes. Clarify with engineering.

## Email Verification Gate
- Router navigation guard (`beforeEach`):
  - If authenticated but `email_verified_at` is null:
    - Allow `/verify-email`, `/logout`
    - Redirect all other routes to `/verify-email`
  - If not authenticated: redirect to `/login`

## Forms & Validation
- Use native HTML5 validation + custom logic
- Show inline error messages below fields
- Disable submit button during API call (loading spinner)
- Generic error display for server errors

## Testing Checklist
- [ ] Registration flow creates account and shows verification page
- [ ] Login redirects unverified users to verification
- [ ] Email verification link works and redirects to onboarding
- [ ] Password reset full flow works
- [ ] Google SSO opens popup and completes login
- [ ] Onboarding wizard connects YouTube channel
- [ ] Router guards enforce auth and verification
- [ ] Token refresh happens transparently on 401
- [ ] Logout clears state and redirects to login

## Files to Create/Modify
- `frontend/src/router/index.ts` — define all routes and guards
- `frontend/src/stores/auth.ts` — Pinia auth store
- `frontend/src/services/api.ts` — Axios instance with interceptors
- `frontend/src/views/auth/*.vue` — auth views
- `frontend/src/views/auth/OnboardingView.vue` — onboarding wizard
- `frontend/src/views/dashboard/DashboardLayout.vue` — main layout
- `frontend/src/views/public/LandingView.vue` — landing page
