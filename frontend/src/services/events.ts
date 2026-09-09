// /api/events — UI telemetry sink. Fire-and-forget by contract:
// tracking must never block or surface errors to the UI path, so this
// module swallows every failure (the endpoint is best-effort signal,
// not user data). Keep payloads to the allowlisted enum in
// internal/api/events_handlers.go — anything free-form is a privacy
// and cardinality bug.

import apiClient from './api'

export type UiEventName = 'digest_section_toggled'
export type DigestSection = 'ideas' | 'questions' | 'insights' | 'health'

interface UiEventProps {
  section: DigestSection
}

export function trackEvent(name: UiEventName, props: UiEventProps): void {
  void apiClient.post('/events', { name, props }).catch(() => {
    // Best-effort: rate-limited or offline UI still works.
  })
}
