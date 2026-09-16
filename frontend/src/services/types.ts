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

export type InsightType = 'video_idea' | 'editing_tip' | 'feature_request' | 'collaboration_ask' | 'improvement_suggestion' | 'content_format_request' | 'other'

export interface Insight {
  type: InsightType
  description: string
  confidence: number
}

export interface Comment {
  id: string
  channel_id: string
  channel_name?: string | null
  channel_display_name?: string | null
  platform_id?: string | null
  platform_channel_id?: string | null
  video_id?: string | null
  video_title?: string | null
  platform_comment_id?: string | null
  author_handle?: string | null
  author_display_name?: string | null
  body: string
  posted_at?: string | null
  processed_at: string | null
  category: Category | null
  topic?: string | null
  actionable_insights: Insight[] | null
  is_starred: boolean
  starred_at?: string | null
}

export interface FeedFilters {
  channels: string[]
  categories: Category[]
  categories_specified?: boolean
  intent?: 'all' | 'ideas' | 'actionable' | null
  insight_type: InsightType | null
  date_from: string | null
  date_to: string | null
  search: string
  include_toxic: boolean
  sort: 'newest' | 'oldest'
}

export interface FeedResponse {
  comments: Comment[]
  total: number
  has_more: boolean
}

export type WindowKey = '7d' | '30d' | '90d' | 'custom' | 'all'

export interface InsightDistributionItem {
  type: InsightType
  count: number
}

export interface InsightAggregatesResponse {
  window: WindowKey
  channel_id: string | null
  distribution: InsightDistributionItem[]
  top_insights: Array<{ type: InsightType; description: string; count: number }>
  total_insights: number
}

export interface ToxicityBucket {
  bucket: string
  filtered: number
  total: number
  filtered_pct: number
}

export interface ToxicityResponse {
  window: WindowKey
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
  top_actionable_insights: Array<{ type: InsightType; description: string; count: number }>
  health_score: number | null
}

export type Plan = 'free' | 'creator' | 'studio' | 'agency'
export type BillingInterval = 'monthly' | 'annual'

export interface BillingStatus {
  plan: Plan | string
  interval: BillingInterval | null
  channels_used: number
  channels_limit: number
  channels_unlimited: boolean
  comments_used: number
  comments_limit: number
  period_end: string | null
  overage_cost_usd?: number | null
  has_payment_method?: boolean
  /** ISO timestamp when the free-trial window ends, when a trial is active. */
  trial_ends_at?: string | null
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
  /** Present on recent-exports list rows. */
  format?: string | null
  created_at?: string | null
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
  /** Set while an email change awaits verification (fix-frontend-plumbing). */
  pending_email?: string | null
}

export type UserType = 'creator' | 'agency' | 'brand' | 'other'
export type PrimaryGoal =
  | 'understand_audience'
  | 'generate_content_ideas'
  | 'improve_retention'
  | 'win_sponsorships'
  | 'reduce_toxicity'
  | 'report_to_clients'
  | 'other'
export type PrimaryPlatform = 'youtube' | 'tiktok' | 'instagram' | 'other'
export type AudienceSizeBucket =
  | 'under_1k'
  | '1k_10k'
  | '10k_50k'
  | '50k_500k'
  | '500k_1m'
  | 'over_1m'
  | 'prefer_not_to_say'
export type WeeklyCommentVolumeBucket =
  | 'under_100'
  | '100_1k'
  | '1k_10k'
  | '10k_100k'
  | 'over_100k'
  | 'prefer_not_to_say'
export type SuccessMetric =
  | 'views'
  | 'watch_time'
  | 'subscribers'
  | 'engagement'
  | 'sentiment'
  | 'sponsor_deals'
  | 'product_sales'
  | 'client_reports'
  | 'other'
export type ProfileRole = 'owner' | 'strategist' | 'analyst' | 'moderator' | 'viewer' | 'other'
export type TeamSizeBucket = 'solo' | '2_5' | '6_20' | '21_plus' | 'prefer_not_to_say'
export type PublishingFrequencyBucket =
  | 'daily'
  | 'several_weekly'
  | 'weekly'
  | 'biweekly'
  | 'monthly'
  | 'irregular'
  | 'prefer_not_to_say'
export type InsightPriority =
  | 'questions'
  | 'content_ideas'
  | 'criticism'
  | 'toxicity'
  | 'collaboration'
  | 'sentiment'
export type CurrentWorkflow =
  | 'native_tools'
  | 'spreadsheets'
  | 'moderation_tool'
  | 'agency_dashboard'
  | 'none'
  | 'other'
export type BiggestPain =
  | 'too_many_comments'
  | 'missed_feedback'
  | 'toxicity'
  | 'reporting_time'
  | 'unclear_content_plan'
  | 'team_visibility'
  | 'other'

export interface CreatorProfile {
  user_id?: string
  display_name?: string | null
  user_type?: UserType | null
  primary_goal?: PrimaryGoal | null
  primary_platform?: PrimaryPlatform | null
  content_topics?: string[]
  audience_size_bucket?: AudienceSizeBucket | null
  weekly_comment_volume_bucket?: WeeklyCommentVolumeBucket | null
  success_metric?: SuccessMetric | null
  role?: ProfileRole | null
  team_size_bucket?: TeamSizeBucket | null
  publishing_frequency_bucket?: PublishingFrequencyBucket | null
  insight_priorities?: InsightPriority[]
  current_workflow?: CurrentWorkflow | null
  biggest_pain?: BiggestPain | null
  created_at?: string | null
  updated_at?: string | null
}
