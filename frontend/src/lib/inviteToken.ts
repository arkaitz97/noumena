// Pending workspace-invite token.
//
// Stored in sessionStorage (per-tab, cleared on success or terminal failure)
// so it survives the full-page login, signup, email-verification, and OAuth
// round-trips without ever appearing in an authenticated request URL.
export const PENDING_INVITE_TOKEN_KEY = 'noumena_pending_invite_token'

export function getPendingInviteToken(): string | null {
  try {
    return sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY)
  } catch {
    return null
  }
}

export function setPendingInviteToken(token: string): void {
  try {
    sessionStorage.setItem(PENDING_INVITE_TOKEN_KEY, token)
  } catch {
    // Storage unavailable (private mode) — the flow retries from the URL.
  }
}

export function clearPendingInviteToken(): void {
  try {
    sessionStorage.removeItem(PENDING_INVITE_TOKEN_KEY)
  } catch {
    // no-op
  }
}
