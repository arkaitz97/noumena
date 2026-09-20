// Mock API harness — intercepts every `/api/*` and `/auth/*` request in the
// browser and answers from the per-test MockState. This is what makes the
// suite deterministic: no real backend, database, Stripe, or OAuth provider
// is ever contacted, yet the SPA runs its complete real code path (axios
// interceptors, Pinia stores, router guards, polling).
//
// The handlers mirror the contract documented in docs/api/ and the typed
// wrappers in src/services/*. If a wrapper changes its URL or body shape,
// update the corresponding handler here — the specs themselves stay stable
// because they assert on user-visible outcomes, not wire formats.
import type { Page, Route } from '@playwright/test'
import type { Comment } from '../../src/services/types'
import { sessionUser, starredComments, type MockChannel, type MockState } from './seed'

const JSON_CT = 'application/json'

/** Fulfill with JSON. Returns true so callers can `return respond(...)` —
 *  in the dispatch functions the boolean means "request handled". */
function respond(route: Route, status: number, body: unknown, headers: Record<string, string> = {}): true {
  void route.fulfill({
    status,
    contentType: JSON_CT,
    headers,
    body: JSON.stringify(body),
  })
  return true
}

function noContent(route: Route): true {
  void route.fulfill({ status: 204 })
  return true
}

/** Yield the pathname without query string, e.g. `/api/comments/feed`. */
function pathOf(route: Route): string {
  return new URL(route.request().url()).pathname
}

function queryOf(route: Route): URLSearchParams {
  return new URL(route.request().url()).searchParams
}

function bodyOf(route: Route): Record<string, unknown> | null {
  const raw = route.request().postData()
  if (!raw) return null
  try {
    return JSON.parse(raw) as Record<string, unknown>
  } catch {
    return null
  }
}

// ---------------------------------------------------------------------------
// Feed filtering / pagination (mirrors internal/api/comment_handlers.go)
// ---------------------------------------------------------------------------

function applyFeedQuery(comments: Comment[], query: URLSearchParams): Comment[] {
  let out = [...comments]
  const channels = query.get('channels')?.split(',').filter(Boolean) ?? []
  if (channels.length > 0) out = out.filter((c) => channels.includes(c.channel_id))
  const hasCategories = query.has('categories')
  const categories = query.get('categories')?.split(',').filter(Boolean) ?? []
  if (hasCategories && categories.length === 0) return []
  if (categories.length > 0) out = out.filter((c) => c.category !== null && categories.includes(c.category))
  const insightType = query.get('insight_type')
  if (insightType) {
    out = out.filter((c) =>
      (c.actionable_insights ?? []).some((i) => i.type === insightType),
    )
  }
  const intent = query.get('intent')
  if (intent === 'ideas') {
    out = out.filter((c) => c.category === 'content_idea' || (c.actionable_insights ?? []).some((i) => i.type === 'video_idea'))
  } else if (intent === 'actionable') {
    out = out.filter((c) => (c.actionable_insights ?? []).length > 0)
  }
  const search = (query.get('search') ?? '').trim().toLowerCase()
  if (search) {
    out = out.filter(
      (c) =>
        c.body.toLowerCase().includes(search) ||
        (c.author_display_name ?? '').toLowerCase().includes(search) ||
        (c.author_handle ?? '').toLowerCase().includes(search),
    )
  }
  const includeToxic = query.get('include_toxic') === 'true'
  if (!includeToxic) {
    out = out.filter((c) => c.category !== 'toxic_criticism' && c.category !== 'spam')
  }
  const sort = query.get('sort')
  if (sort === 'oldest') {
    out.sort((a, b) => (a.posted_at ?? '').localeCompare(b.posted_at ?? ''))
  } else {
    out.sort((a, b) => (b.posted_at ?? '').localeCompare(a.posted_at ?? ''))
  }
  return out
}

function pageOf(comments: Comment[], query: URLSearchParams): { comments: Comment[]; total: number; has_more: boolean } {
  const offset = Number(query.get('offset') ?? 0)
  const limit = Number(query.get('limit') ?? 20)
  const page = comments.slice(offset, offset + limit)
  return { comments: page, total: comments.length, has_more: offset + page.length < comments.length }
}

// ---------------------------------------------------------------------------
// Auth surface
// ---------------------------------------------------------------------------

function handleAuth(route: Route, state: MockState): boolean {
  const method = route.request().method()
  const path = pathOf(route)
  const user = sessionUser(state)

  switch (`${method} ${path}`) {
    case 'POST /auth/refresh': {
      if (state.refreshFails || !user) return respond(route, 401, { error: 'no session' })
      return respond(route, 200, { access_token: 'mock-access-token' })
    }
    case 'POST /auth/login': {
      const body = bodyOf(route) ?? {}
      const email = String(body.email ?? '').toLowerCase()
      const password = String(body.password ?? '')
      const found = state.users.find((u) => u.email === email)
      if (!found || found.password !== password) {
        return respond(route, 401, { error: 'invalid credentials' })
      }
      state.sessionUserEmail = found.email
      return respond(route, 200, { access_token: 'mock-access-token', message: 'ok' })
    }
    case 'POST /auth/register': {
      const body = bodyOf(route) ?? {}
      const email = String(body.email ?? '')
      if (state.users.some((u) => u.email === email)) {
        return respond(route, 409, { error: 'email already registered' })
      }
      state.users.push({
        id: `u-${state.users.length + 1}`,
        email,
        password: String(body.password ?? ''),
        email_verified_at: null,
        subscription_plan: 'free',
        subscription_status: 'active',
      })
      state.sessionUserEmail = email
      state.channels = []
      return respond(route, 201, { access_token: 'mock-access-token', message: 'created' })
    }
    case 'POST /auth/logout':
    case 'POST /auth/logout-all':
      state.sessionUserEmail = null
      return respond(route, 200, { message: 'logged out' })
    case 'POST /auth/verify-email': {
      if (user) user.email_verified_at = new Date().toISOString()
      return respond(route, 200, { message: 'verified' })
    }
    case 'POST /auth/forgot-password':
      return respond(route, 200, { message: 'reset link sent' })
    case 'POST /auth/resend-verification':
      return respond(route, 200, { message: 'verification email sent' })
    case 'POST /auth/reset-password':
      return respond(route, 200, { message: 'password updated' })
    case 'POST /auth/password': {
      const body = bodyOf(route) ?? {}
      if (!user || body.current_password !== user.password) {
        return respond(route, 401, { error: 'current password is incorrect' })
      }
      user.password = String(body.new_password ?? '')
      return respond(route, 200, { message: 'password updated', access_token: 'mock-access-token' })
    }
    case 'GET /auth/google':
    case 'GET /auth/google?json=1':
      return respond(route, 200, { auth_url: 'https://accounts.google.com/o/oauth2/v2/auth?mock=1', state: 'mock-state' })
    default:
      return false
  }
}

// ---------------------------------------------------------------------------
// API surface
// ---------------------------------------------------------------------------

function handleApi(route: Route, state: MockState): boolean {
  const method = route.request().method()
  const path = pathOf(route)
  const query = queryOf(route)
  const user = sessionUser(state)

  // ---- profile -----------------------------------------------------------
  if (path === '/api/user/me') {
    if (!user) return respond(route, 401, { error: 'unauthorized' })
    return respond(route, 200, {
      id: user.id,
      email: user.email,
      email_verified: user.email_verified_at !== null,
      email_verified_at: user.email_verified_at,
      subscription_plan: user.subscription_plan,
      subscription_status: user.subscription_status,
      created_at: new Date().toISOString(),
    })
  }

  if (method === 'PATCH' && path === '/api/user/me') {
    return respond(route, 200, { message: 'profile updated' })
  }

  // ---- workspaces / roles --------------------------------------------------
  if (path === '/api/workspaces/active/role') {
    return respond(route, 200, { role: 'admin' })
  }
  if (path === '/api/workspaces/mine' || path === '/api/workspaces') {
    return respond(route, 200, [
      { id: 'ws-1', name: 'My Workspace', role: 'admin', is_active: true },
    ])
  }

  // ---- invites -------------------------------------------------------------
  // Token contract: `valid` → success, `seatfull` → 409 seat cap, anything
  // else → the generic 410 the backend returns for unknown/expired/consumed.
  if (path === '/api/invites/preview') {
    const token = query.get('token')
    if (token === 'valid' || token === 'seatfull') {
      return respond(route, 200, { workspace_name: 'Studio', email: 'invitee@example.com', role: 'viewer' })
    }
    return respond(route, 410, { error: 'invite expired or already used' })
  }
  if (method === 'POST' && path === '/api/invites/accept') {
    const token = String(bodyOf(route)?.token ?? '')
    if (token === 'valid') {
      return respond(route, 200, { workspace_id: 'ws-1', workspace_name: 'Studio', role: 'viewer' })
    }
    if (token === 'seatfull') {
      return respond(route, 409, { error: 'seat limit reached, upgrade your plan' })
    }
    return respond(route, 410, { error: 'invite expired or already used' })
  }

  // ---- channels ------------------------------------------------------------
  if (path === '/api/channels') {
    if (method === 'GET') return respond(route, 200, state.channels)
  }
  const channelIdMatch = path.match(/^\/api\/channels\/([^/]+)\/(status|sync-status|pause|resume|backfill-window|custom-prompt)$/)
  if (channelIdMatch) {
    const [, id, action] = channelIdMatch
    const channel = state.channels.find((c) => c.id === id)
    if (!channel) return respond(route, 404, { error: 'channel not found' })
    switch (action) {
      case 'status':
        return respond(route, 200, state.channelStatus[id])
      case 'sync-status':
        return respond(route, 200, {
          last_synced_at: channel.last_synced_at,
          next_sync_at: null,
          backfill_progress_percent: state.backfillProgress,
          recent_error: state.backfillError,
        })
      case 'pause':
        channel.tracking_status = 'paused'
        state.channelStatus[id] = { ...state.channelStatus[id], tracking_status: 'paused' }
        return respond(route, 200, { message: 'paused' })
      case 'resume':
        channel.tracking_status = 'active'
        state.channelStatus[id] = { ...state.channelStatus[id], tracking_status: 'active' }
        return respond(route, 200, { message: 'resumed' })
      case 'backfill-window':
        channel.backfill_days = Number(bodyOf(route)?.days ?? channel.backfill_days)
        return respond(route, 200, { message: 'updated' })
      default:
        return respond(route, 200, { message: 'ok' })
    }
  }
  if (method === 'DELETE' && /^\/api\/channels\/[^/]+$/.test(path)) {
    const id = path.split('/').pop()!
    state.channels = state.channels.filter((c) => c.id !== id)
    delete state.channelStatus[id]
    return respond(route, 200, { message: 'removed' })
  }
  if (path === '/api/channels/connect/youtube') {
    return respond(route, 200, { auth_url: 'https://accounts.google.com/o/oauth2/v2/auth?mock=1', state: 'mock-youtube-state' })
  }
  if (path === '/api/channels/exchange/youtube') {
    const body = bodyOf(route) ?? {}
    const channel: MockChannel = {
      id: 'ch-new',
      platform: 'youtube',
      platform_channel_id: 'yt-new',
      display_name: 'New Channel',
      tracking_status: 'active',
      token_priority: 0,
      backfill_days: Number(body.backfill_days ?? 90),
      last_synced_at: new Date().toISOString(),
    }
    // The exchange is the source of truth for "a channel was connected":
    // push it so refreshChannelsCount and the dashboard guard see it.
    state.channels.push(channel)
    state.channelStatus['ch-new'] = {
      last_synced_at: channel.last_synced_at!,
      next_sync_at: null,
      comment_count: 0,
      is_primary_token_holder: true,
      tracking_status: 'active',
      backfill_progress_percent: state.backfillProgress,
      recent_error: state.backfillError,
    }
    return respond(route, 200, channel)
  }

  // ---- comments: feed / saved / star / unstar -------------------------------
  if (path === '/api/comments/feed') {
    if (state.failNext && path.includes(state.failNext.urlPart)) {
      const f = state.failNext
      state.failNext = null
      return respond(route, f.status, f.body ?? { error: 'forced failure' })
    }
    if (state.feedTimesOut) {
      return respond(route, 408, { error: 'query timeout', code: 'feed_query_timeout' })
    }
    if (state.feedServerError) return respond(route, 500, { error: 'boom' })
    return respond(route, 200, pageOf(applyFeedQuery(state.comments, query), query))
  }
  if (path === '/api/comments/saved') {
    return respond(route, 200, pageOf(applyFeedQuery(starredComments(state), query), query))
  }
  const starMatch = path.match(/^\/api\/comments\/([^/]+)\/star$/)
  if (starMatch) {
    const id = starMatch[1]
    const comment = state.comments.find((c) => c.id === id)
    if (!comment) return respond(route, 404, { error: 'comment not found' })
    if (method === 'POST') {
      comment.is_starred = true
      comment.starred_at = new Date().toISOString()
      return respond(route, 200, { message: 'starred' })
    }
    if (method === 'DELETE') {
      comment.is_starred = false
      comment.starred_at = null
      return respond(route, 200, { message: 'unstarred' })
    }
  }

  // ---- exports ---------------------------------------------------------------
  if (path === '/api/comments/export' && method === 'POST') {
    state.calls.push({ method, path, body: bodyOf(route) ?? {} })
    if (state.exportFails) return respond(route, 500, { error: 'export unavailable' })
    const body = bodyOf(route) ?? {}
    const format = String(body.format ?? 'csv')
    if (state.exportMode === 'sync') {
      return respond(route, 200, {
        download_url: '/api/comments/export/job-sync/download',
        expires_at: new Date(Date.now() + 24 * 3600_000).toISOString(),
      })
    }
    const jobId = `job-${state.nextExportJobId++}`
    state.exportJobs[jobId] = {
      job_id: jobId,
      status: 'queued',
      format,
      created_at: new Date().toISOString(),
    }
    return respond(route, 202, { job_id: jobId })
  }
  const exportStatusMatch = path.match(/^\/api\/comments\/export\/([^/]+)\/status$/)
  if (exportStatusMatch) {
    const job = state.exportJobs[exportStatusMatch[1]]
    if (!job) return respond(route, 404, { error: 'job not found' })
    // Deterministic progression: first poll completes the job so the
    // store's 5s poll loop converges on the first tick.
    if (job.status !== 'completed' && job.status !== 'failed') {
      job.status = 'completed'
      job.download_url = `/api/comments/export/${job.job_id}/download`
      job.expires_at = new Date(Date.now() + 24 * 3600_000).toISOString()
    }
    return respond(route, 200, job)
  }
  if (path === '/api/comments/export/recent') {
    return respond(route, 200, Object.values(state.exportJobs).slice(-5))
  }

  // ---- insights ----------------------------------------------------------------
  if (path === '/api/insights/actionable-insights') {
    const withInsights = state.comments.filter((c) => (c.actionable_insights ?? []).length > 0)
    return respond(route, 200, {
      window: query.get('window') ?? '7d',
      channel_id: query.get('channel_id') ?? null,
      total_insights: withInsights.length,
      distribution: [
        { type: 'video_idea', count: 3 },
        { type: 'feature_request', count: 2 },
      ],
      top_insights: [
        { type: 'video_idea', description: 'Turn this into a dedicated tutorial video.', count: 3 },
      ],
    })
  }
  if (path === '/api/insights/toxicity-shield') {
    return respond(route, 200, {
      window: '7d',
      channel_id: null,
      filtered_pct: 8.3,
      filtered_comments: 12,
      total_comments: 144,
      series: [
        { bucket: 'Mon', filtered: 1, total: 22, filtered_pct: 4.5 },
        { bucket: 'Tue', filtered: 3, total: 21, filtered_pct: 14.3 },
      ],
    })
  }
  if (path === '/api/insights/trends') {
    return respond(route, 200, {
      window: '7d',
      channel_id: null,
      top_topics: [
        { topic: 'sourdough', count: 7 },
        { topic: 'undervolting', count: 5 },
      ],
      top_categories: [{ category: 'praise', count: 12 }],
    })
  }
  if (path === '/api/insights/sentiment') {
    return respond(route, 200, {
      days: Number(query.get('days') ?? 7),
      channel_id: query.get('channel_id') ?? null,
      series: [
        { date: '2026-08-01', negative: 1, neutral: 2, positive: 3, score: 0.25 },
        { date: '2026-08-02', negative: 0, neutral: 1, positive: 2, score: 0.66 },
      ],
    })
  }
  if (path === '/api/insights/digest/latest' || path === '/api/digests/latest') {
    return respond(route, 200, {
      id: 'digest-1',
      week_start: '2026-08-24',
      week_end: '2026-08-30',
      top_ideas: [{ title: 'Sourdough deep dive', source_count: 7 }],
      top_questions: [{ title: 'High-altitude baking?', source_count: 4 }],
      top_actionable_insights: [{ type: 'video_idea', description: 'Tutorial follow-up', count: 3 }],
      health_score: 82,
    })
  }

  // ---- UI telemetry ---------------------------------------------------------------
  if (path === '/api/events' && method === 'POST') {
    return noContent(route)
  }

  // ---- billing ------------------------------------------------------------------
  if (path === '/api/billing/status') {
    return respond(route, 200, state.billing)
  }
  if (path === '/api/billing/usage') {
    return respond(route, 200, { periods: state.usage })
  }
  if (path === '/api/billing/checkout' && method === 'POST') {
    state.calls.push({ method, path, body: bodyOf(route) ?? {} })
    return respond(route, 200, { url: 'https://checkout.stripe.com/mock-session' })
  }
  if (path === '/api/billing/portal' && method === 'GET') {
    if (!state.billing.has_payment_method) {
      return respond(route, 409, { error: 'no stripe customer' })
    }
    return respond(route, 200, { url: 'https://billing.stripe.com/mock-portal' })
  }

  // ---- lifecycle / misc -------------------------------------------------------------
  if (path === '/api/user/token-expiry') {
    return respond(route, 200, { channels: [] })
  }
  if (path === '/api/user/rate-limit-status') {
    return respond(route, 200, { limited: false, retry_after: 0 })
  }
  if (path === '/api/user/data-export' && method === 'POST') {
    const job: { job_id: string; status: string } = { job_id: 'gdpr-1', status: 'processing' }
    return respond(route, 201, job)
  }
  const gdprStatusMatch = path.match(/^\/api\/user\/data-export\/([^/]+)\/status$/)
  if (gdprStatusMatch) {
    return respond(route, 200, {
      job_id: gdprStatusMatch[1],
      status: 'completed',
      download_url: '/api/exports/gdpr-1/download',
      expires_at: new Date(Date.now() + 24 * 3600_000).toISOString(),
    })
  }
  if (path === '/api/user/account' && method === 'DELETE') {
    state.sessionUserEmail = null
    return respond(route, 200, { message: 'deletion scheduled' })
  }
  if (path === '/api/user/account/cancel-deletion' && method === 'POST') {
    return respond(route, 200, { message: 'deletion cancelled' })
  }

  // ---- unmocked: fail loudly so tests never silently pass on stale mocks ----
  return respond(route, 501, { error: `unmocked ${method} ${path} — update mock.ts` })
}

// ---------------------------------------------------------------------------
// Installer
// ---------------------------------------------------------------------------

function handlerFor(state: MockState): (route: Route) => void {
  return (route) => {
    const path = pathOf(route)
    const handled = path.startsWith('/auth/') ? handleAuth(route, state) : handleApi(route, state)
    if (!handled && path.startsWith('/auth/')) {
      respond(route, 501, { error: `unmocked auth ${route.request().method()} ${path}` })
    }
  }
}

/** Install the mock on a page. Call once per test (fresh state, fresh page).
 *
 *  NOTE: the patterns are anchored at the path start (/api/, /auth/) — a
 *  bare double-star glob would also hijack Vite's own lazy-loaded modules
 *  under src/views/auth/ and kill the SPA. */
export async function installMockApi(page: Page, state: MockState): Promise<void> {
  await page.route(/^(?:https?:\/\/[^/]+)?\/(?:api|auth)\//, handlerFor(state))
}
