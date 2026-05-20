// Shared TypeScript types for the auth + channels API surface.

export interface User {
  id: string
  email: string
  /**
   * Backend `/api/user/me` returns `email_verified: boolean`. We normalize to
   * an ISO timestamp when known, or `null` when not. The store treats
   * `email_verified_at !== null` as verified.
   */
  email_verified_at: string | null
  subscription_plan?: string
  subscription_status?: string
  created_at?: string
}

export interface LoginResponse {
  access_token: string
  message?: string
}

export interface RegisterResponse {
  access_token: string
  message?: string
}

export interface RefreshResponse {
  access_token: string
}

export interface Channel {
  id: string
  platform: string
  platform_channel_id: string
  display_name: string
  tracking_status: string
  token_priority: number
  last_synced_at?: string | null
  backfill_days: number
  token_expires_at?: string | null
  token_expiry_notified_at?: string | null
}

export interface SyncStatus {
  last_synced_at: string | null
  next_sync_at: string | null
  backfill_progress_percent: number | null
  recent_error: { at: string; message: string } | null
}

export interface ConnectChannelResponse {
  auth_url: string
  state: string
}

export interface ExchangeChannelResponse {
  id: string
  platform_channel_id: string
  display_name: string
  tracking_status: string
  token_priority: number
  backfill_days: number
  token_expires_at?: string | null
}

export interface ApiErrorBody {
  error?: string
  code?: string
  message?: string
}

// ---------------------------------------------------------------------------
// Insights / comments / billing / lifecycle response shapes (Phase 10).
// ---------------------------------------------------------------------------

import type { Category } from '../lib/categories'

export interface Comment {
  id: string
  channel_id: string
  channel_display_name?: string | null
  platform_comment_id?: string | null
  author_handle?: string | null
  author_display_name?: string | null
  body: string
  posted_at?: string | null
  processed_at: string | null
  category: Category | null
  topic?: string | null
  sentiment_score: number | null
  is_starred: boolean
  starred_at?: string | null
}

export interface FeedFilters {
  channels: string[]
  categories: Category[]
  sentiment_min: number | null
  sentiment_max: number | null
  date_from: string | null
  date_to: string | null
  search: string
  include_toxic: boolean
  sort: 'newest' | 'sentiment_desc' | 'sentiment_asc'
}

export interface FeedResponse {
  comments: Comment[]
  total: number
  has_more: boolean
}

export type WindowKey = '7d' | '30d' | '90d'

export interface SentimentBucket {
  bucket: string // ISO date or window slot
  avg: number
  count: number
  positive_pct?: number
  negative_pct?: number
  neutral_pct?: number
}

export interface SentimentResponse {
  window: WindowKey
  channel_id: string | null
  series: SentimentBucket[]
  avg: number
  count: number
}

export interface ToxicityBucket {
  bucket: string
  filtered: number
  total: number
  filtered_pct: number
}

export interface ToxicityResponse {
  window: '7d' | '30d'
  channel_id: string | null
  filtered_pct: number
  filtered_comments: number
  total_comments: number
  series?: ToxicityBucket[]
}

export interface TopicRow {
  topic: string
  count: number
}

export interface CategoryRow {
  category: Category
  count: number
}

export interface TrendsResponse {
  window: WindowKey
  channel_id: string | null
  top_topics: TopicRow[]
  top_categories: CategoryRow[]
}

export interface WeeklyDigest {
  id?: string
  week_start: string
  week_end: string
  top_ideas: Array<{ title: string; summary?: string; source_count?: number }>
  top_questions: Array<{ title: string; summary?: string; source_count?: number }>
  sentiment_trend: { avg_7d: number; avg_30d: number; delta?: number; direction?: 'up' | 'down' | 'flat' }
  health_score: number | null
}

export type Plan = 'free' | 'creator' | 'studio' | 'agency'
export type BillingInterval = 'monthly' | 'annual'

export interface BillingStatus {
  plan: Plan | string
  interval: BillingInterval | null
  channels_used: number
  channels_limit: number
  comments_used: number
  comments_limit: number
  period_end: string | null
  overage_cost_usd?: number | null
  has_payment_method?: boolean
}

export type ExportFormat = 'csv' | 'json'

export type ExportStatus = 'queued' | 'processing' | 'exporting' | 'completed' | 'failed'

export interface ExportJob {
  job_id: string
  status: ExportStatus
  download_url?: string | null
  export_url?: string | null
  expires_at?: string | null
  error?: string | null
}

export interface ChannelStatus {
  last_synced_at: string | null
  next_sync_at: string | null
  comment_count: number | null
  is_primary_token_holder: boolean
  tracking_status: 'active' | 'paused' | string
  backfill_progress_percent?: number | null
  recent_error?: { at: string; message: string } | null
}

export interface UserProfile {
  id: string
  email: string
  email_verified_at: string | null
  subscription_plan?: string
  subscription_status?: string
  created_at?: string
  weekly_digest_opt_in?: boolean | null
  deletion_requested_at?: string | null
}
