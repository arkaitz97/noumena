// Fixture data + MockState factory.
//
// Every test gets a brand-new MockState via the `mockState` fixture — no
// shared mutable state across tests, so tests are fully isolated and
// re-runnable. All identifiers are synthetic (`c1`, `ch1`, …) and MUST NOT
// be asserted in specs: locators resolve through visible text/roles, which
// is what makes the suite survive backend or seed refactors.
import type {
  BillingStatus,
  Channel,
  ChannelStatus,
  Comment,
  InsightType,
} from '../../src/services/types'
import type { UsagePeriod } from '../../src/services/billing'

export const VERIFIED_EMAIL = 'creator@noumena.test'
export const UNVERIFIED_EMAIL = 'fresh@noumena.test'
export const TEST_PASSWORD = 'Sup3rSecret!'

export interface MockUser {
  id: string
  email: string
  password: string
  email_verified_at: string | null
  subscription_plan: string
  subscription_status: string
}

export interface MockChannel extends Channel {}

export interface MockComment extends Comment {}

export interface MockExportJob {
  job_id: string
  status: 'queued' | 'processing' | 'exporting' | 'completed' | 'failed'
  download_url?: string
  expires_at?: string
  format?: string
  error?: string
  created_at?: string
}

export type SessionProfile = 'guest' | 'unverified' | 'no-channels' | 'verified'

export interface MockTrial {
  active: boolean
  ends_at: string
  days_remaining: number
}

export interface MockState {
  users: MockUser[]
  /** Email of the session the mock considers logged in (null = guest). */
  sessionUserEmail: string | null
  channels: MockChannel[]
  /** Per-channel status detail, keyed by channel id. */
  channelStatus: Record<string, ChannelStatus>
  comments: MockComment[]
  exportJobs: Record<string, MockExportJob>
  billing: BillingStatus & { trial?: MockTrial | null }
  usage: UsagePeriod[]
  nextExportJobId: number
  /** 'sync' → POST /comments/export answers 200 with a URL. */
  exportMode: 'sync' | 'async'
  /** When true, POST /comments/export answers 500 (error path). */
  exportFails: boolean
  /** Retry-After seconds used by the mock when returning 429. */
  rateLimitRetryAfter: number
  /** When true, POST /auth/refresh answers 401 (expired session). */
  refreshFails: boolean
  /** When true, GET /api/comments/feed answers 408 (query timeout). */
  feedTimesOut: boolean
  /** When true, GET /api/comments/feed answers 500. */
  feedServerError: boolean
  /** When set, the next matching request is failed once with this status. */
  failNext: { urlPart: string; status: number; body?: Record<string, unknown> } | null
  /** Recorded requests for behavioural assertions (checkout calls etc.). */
  calls: Array<{ method: string; path: string; body?: unknown }>
  /** Progress % returned by sync-status during onboarding polling. */
  backfillProgress: number
  /** Recent-error payload returned by sync-status (null = healthy). */
  backfillError: { at: string; message: string } | null
}

const DAY = 86_400_000
const iso = (daysAgo: number): string => new Date(Date.now() - daysAgo * DAY).toISOString()

function makeChannel(id: string, displayName: string, opts: Partial<MockChannel> = {}): MockChannel {
  return {
    id,
    platform: 'youtube',
    platform_channel_id: `yt-${id}`,
    display_name: displayName,
    tracking_status: 'active',
    token_priority: 0,
    backfill_days: 90,
    last_synced_at: iso(1),
    ...opts,
  }
}

function makeComment(
  id: string,
  channelId: string,
  channelName: string,
  body: string,
  opts: Partial<MockComment> = {},
): MockComment {
  return {
    id,
    channel_id: channelId,
    channel_display_name: channelName,
    platform_id: `yt-${channelId}`,
    video_id: `vid_${id}`,
    video_title: `Test video ${id.toUpperCase()}`,
    author_handle: `author_${id}`,
    author_display_name: `Author ${id.toUpperCase()}`,
    body,
    posted_at: iso(2),
    processed_at: iso(1),
    category: 'other',
    topic: null,
    actionable_insights: null,
    is_starred: false,
    starred_at: null,
    ...opts,
  }
}

const INSIGHTS: Array<{ type: InsightType; description: string; confidence: number }> = [
  { type: 'video_idea', description: 'Turn this into a dedicated tutorial video.', confidence: 0.9 },
  { type: 'feature_request', description: 'Viewers want a beginner-friendly walkthrough.', confidence: 0.82 },
]

function buildComments(): MockComment[] {
  const rows: Array<[string, string, string, string, Partial<MockComment>]> = [
    ['c01', 'ch1', 'Tech Reviews', 'Love the deep dive on the new GPU — benchmark section was gold.', { category: 'praise' }],
    ['c02', 'ch1', 'Tech Reviews', 'Could you compare this against last years model in the next video?', { category: 'question' }],
    ['c03', 'ch1', 'Tech Reviews', 'You should make a follow-up covering undervolting settings.', { category: 'content_idea', topic: 'undervolting', actionable_insights: INSIGHTS }],
    ['c04', 'ch1', 'Tech Reviews', 'Happy to collab on a mechanical keyboard build if you are interested.', { category: 'collaboration_ask' }],
    ['c05', 'ch1', 'Tech Reviews', 'The audio mix was a bit quiet during the hands-on segment.', { category: 'constructive_criticism' }],
    ['c06', 'ch2', 'Cooking Lab', 'The 72-hour dough recipe changed my bread game entirely.', { category: 'praise' }],
    ['c07', 'ch2', 'Cooking Lab', 'Does the sourdough method work at high altitude?', { category: 'question', topic: 'sourdough' }],
    ['c08', 'ch2', 'Cooking Lab', 'A video on knife sharpening basics would be awesome.', { category: 'content_idea', topic: 'knife sharpening', actionable_insights: INSIGHTS }],
    ['c09', 'ch2', 'Cooking Lab', 'Your plating tutorials inspired our restaurant menu redesign.', { category: 'collaboration_ask' }],
    ['c10', 'ch2', 'Cooking Lab', 'The fermentation video skipped the temperature control details.', { category: 'constructive_criticism' }],
    ['c11', 'ch1', 'Tech Reviews', 'Worst clickbait on this platform, unsubscribe.', { category: 'toxic_criticism' }],
    ['c12', 'ch1', 'Tech Reviews', 'Check out my crypto channel link below!!!', { category: 'spam' }],
    ['c13', 'ch1', 'Tech Reviews', 'Still processing comment', { processed_at: null }],
    ['c14', 'ch2', 'Cooking Lab', 'Add a printable version of the recipe card please.', { category: 'content_idea' }],
    ['c15', 'ch2', 'Cooking Lab', 'Which cast iron pan do you recommend for beginners?', { category: 'question' }],
    ['c16', 'ch1', 'Tech Reviews', 'The 4K capture test at the end was really clever.', { category: 'praise' }],
    ['c17', 'ch1', 'Tech Reviews', 'Please test Linux compatibility in the next review.', { category: 'question', topic: 'linux' }],
    ['c18', 'ch2', 'Cooking Lab', 'Great weeknight meal ideas for busy families.', { category: 'praise' }],
    ['c19', 'ch2', 'Cooking Lab', 'A sous-vide basics episode would fill a real gap.', { category: 'content_idea' }],
    ['c20', 'ch1', 'Tech Reviews', 'The battery section felt rushed compared to the rest.', { category: 'constructive_criticism' }],
    ['c21', 'ch2', 'Cooking Lab', 'Do you ship the spice blend internationally?', { category: 'question' }],
    ['c22', 'ch1', 'Tech Reviews', 'Great collab potential with your channel, DM me.', { category: 'collaboration_ask' }],
    ['c23', 'ch2', 'Cooking Lab', 'That tart looks like a professional bakery made it.', { category: 'praise' }],
    ['c24', 'ch1', 'Tech Reviews', 'Totally agree with the verdict on thermals.', { category: 'praise' }],
  ]
  return rows.map(([id, ch, name, body, opts]) => makeComment(id, ch, name, body, opts))
}

/**
 * Fresh, fully isolated mock state. Each call returns independent objects
 * so tests can mutate freely without leaking into siblings.
 */
export function createMockState(): MockState {
  return {
    users: [
      {
        id: 'u-verified',
        email: VERIFIED_EMAIL,
        password: TEST_PASSWORD,
        email_verified_at: iso(30),
        subscription_plan: 'free',
        subscription_status: 'active',
      },
      {
        id: 'u-unverified',
        email: UNVERIFIED_EMAIL,
        password: TEST_PASSWORD,
        email_verified_at: null,
        subscription_plan: 'free',
        subscription_status: 'active',
      },
    ],
    sessionUserEmail: null,
    channels: [
      makeChannel('ch1', 'Tech Reviews', { token_priority: 1 }),
      makeChannel('ch2', 'Cooking Lab'),
    ],
    channelStatus: {
      ch1: {
        last_synced_at: iso(1),
        next_sync_at: new Date(Date.now() + 30 * 60 * 1000).toISOString(),
        comment_count: 1400,
        is_primary_token_holder: true,
        tracking_status: 'active',
        backfill_progress_percent: 100,
        recent_error: null,
      },
      ch2: {
        last_synced_at: iso(2),
        next_sync_at: new Date(Date.now() + 30 * 60 * 1000).toISOString(),
        comment_count: 880,
        is_primary_token_holder: false,
        tracking_status: 'active',
        backfill_progress_percent: 100,
        recent_error: null,
      },
    },
    comments: buildComments(),
    exportJobs: {},
    billing: {
      plan: 'free',
      interval: null,
      channels_used: 2,
      channels_limit: 1,
      comments_used: 900,
      comments_limit: 1000,
      period_end: new Date(Date.now() + 20 * DAY).toISOString(),
      overage_cost_usd: null,
      has_payment_method: false,
      trial_ends_at: null,
      trial: null,
    },
    usage: [
      { period_start: '2026-08-01', period_end: '2026-08-31', comments_used: 900, quota: 1000 },
      { period_start: '2026-07-01', period_end: '2026-07-31', comments_used: 610, quota: 1000 },
      { period_start: '2026-06-01', period_end: '2026-06-30', comments_used: 340, quota: 1000 },
    ],
    nextExportJobId: 1,
    exportMode: 'sync',
    exportFails: false,
    rateLimitRetryAfter: 30,
    refreshFails: false,
    feedTimesOut: false,
    feedServerError: false,
    failNext: null,
    calls: [],
    backfillProgress: 100,
    backfillError: null,
  }
}

/**
 * Configure the mock session to one of the router-guard archetypes:
 *   guest        → no session; guards bounce to /login.
 *   unverified   → session, email not verified; guards bounce to /verify-email.
 *   no-channels  → session, verified, no channels; guards bounce to /onboarding.
 *   verified     → full access to /dashboard.
 */
export function applyProfile(state: MockState, profile: SessionProfile): void {
  switch (profile) {
    case 'guest':
      state.sessionUserEmail = null
      break
    case 'unverified': {
      const u = state.users.find((x) => x.email === UNVERIFIED_EMAIL)!
      u.email_verified_at = null
      state.sessionUserEmail = u.email
      state.channels = []
      break
    }
    case 'no-channels': {
      const u = state.users.find((x) => x.email === VERIFIED_EMAIL)!
      u.email_verified_at = iso(30)
      state.sessionUserEmail = u.email
      state.channels = []
      break
    }
    case 'verified':
    default: {
      const u = state.users.find((x) => x.email === VERIFIED_EMAIL)!
      u.email_verified_at = iso(30)
      state.sessionUserEmail = u.email
      break
    }
  }
}

/** The user behind the current mock session (null when guest). */
export function sessionUser(state: MockState): MockUser | null {
  return state.users.find((u) => u.email === state.sessionUserEmail) ?? null
}

/** Comments that the mock considers starred. */
export function starredComments(state: MockState): MockComment[] {
  return state.comments.filter((c) => c.is_starred)
}