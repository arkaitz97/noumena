// Reusable OAuth popup plumbing used by both Google SSO (LoginView /
// SignupView) and the YouTube channel-connect step in the onboarding wizard.
//
// Flow:
//   1. Caller generates `state` (a UUID) and stashes it under
//      `sessionStorage["oauth_state_<provider>"]`.
//   2. Caller opens a popup synchronously inside the user click handler.
//      `window.open` MUST run inside the same call stack as the click to
//      avoid pop-up blockers.
//   3. The backend authorize URL eventually redirects the popup to
//      `${origin}/oauth/callback?code=...&state=...`. The
//      `OAuthCallbackView` component there posts back
//      `{ type: 'oauth-code', provider, code, state }` to `window.opener`
//      and calls `window.close()`.
//   4. This helper resolves with `{ code, state }` after validating the
//      origin and state.
//
// Errors:
//   - Pop-up blocked (`window.open` returned null) → reject with
//     `Error('popup_blocked')`.
//   - Timeout (5 minutes) → reject with `Error('popup_timeout')`.
//   - User closed the popup → reject with `Error('popup_closed')`.

export type OAuthProvider = 'google' | 'youtube'

export interface OAuthResult {
  code: string
  state: string
}

interface OAuthCodeMessage {
  type: 'oauth-code'
  provider: OAuthProvider
  code: string
  state: string
}

const POPUP_W = 500
const POPUP_H = 600
const TIMEOUT_MS = 5 * 60 * 1000
const POLL_CLOSED_MS = 500

export function generateState(provider: OAuthProvider): string {
  const cryptoRef = (globalThis as { crypto?: Crypto }).crypto
  const state =
    cryptoRef && typeof cryptoRef.randomUUID === 'function'
      ? cryptoRef.randomUUID()
      : Math.random().toString(36).slice(2) + Date.now().toString(36)
  sessionStorage.setItem(`oauth_state_${provider}`, state)
  return state
}

function clearStoredState(provider: OAuthProvider): void {
  sessionStorage.removeItem(`oauth_state_${provider}`)
}

export interface OpenPopupOptions {
  /** Pre-generated state token (recommended — call `generateState` first). */
  state?: string
}

/**
 * Open the OAuth popup and resolve when the callback view posts the code
 * back. The opener MUST call this synchronously inside a click handler to
 * avoid popup blockers — i.e. do not `await` anything before invoking
 * `openOAuthPopup`.
 */
export function openOAuthPopup(
  provider: OAuthProvider,
  authUrl: string,
  options: OpenPopupOptions = {},
): Promise<OAuthResult> {
  // Ensure a state token exists in sessionStorage even if the caller didn't
  // explicitly call generateState (defensive).
  if (options.state) {
    sessionStorage.setItem(`oauth_state_${provider}`, options.state)
  } else if (!sessionStorage.getItem(`oauth_state_${provider}`)) {
    generateState(provider)
  }

  const features = `width=${POPUP_W},height=${POPUP_H},menubar=no,toolbar=no,location=yes,status=no,resizable=yes,scrollbars=yes`
  const popup = window.open(authUrl, `oauth_${provider}`, features)
  if (!popup) {
    clearStoredState(provider)
    return Promise.reject(new Error('popup_blocked'))
  }

  return new Promise<OAuthResult>((resolve, reject) => {
    let timeoutId: number | null = null
    let pollId: number | null = null
    let settled = false

    const cleanup = () => {
      if (settled) return
      settled = true
      window.removeEventListener('message', onMessage)
      if (timeoutId !== null) window.clearTimeout(timeoutId)
      if (pollId !== null) window.clearInterval(pollId)
      clearStoredState(provider)
      try {
        if (popup && !popup.closed) popup.close()
      } catch {
        /* ignore — cross-origin */
      }
    }

    const onMessage = (event: MessageEvent) => {
      // Origin check — refuse anything not from our own origin.
      if (event.origin !== window.location.origin) return
      const data = event.data as OAuthCodeMessage | undefined
      if (!data || data.type !== 'oauth-code' || data.provider !== provider) return

      const expected = sessionStorage.getItem(`oauth_state_${provider}`)
      if (!expected || expected !== data.state) {
        cleanup()
        reject(new Error('state_mismatch'))
        return
      }
      cleanup()
      resolve({ code: data.code, state: data.state })
    }

    window.addEventListener('message', onMessage)

    timeoutId = window.setTimeout(() => {
      cleanup()
      reject(new Error('popup_timeout'))
    }, TIMEOUT_MS)

    // Detect user-closed popup.
    pollId = window.setInterval(() => {
      try {
        if (popup.closed) {
          cleanup()
          reject(new Error('popup_closed'))
        }
      } catch {
        /* cross-origin while popup is on Google's domain — ignore */
      }
    }, POLL_CLOSED_MS)
  })
}
