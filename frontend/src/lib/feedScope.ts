import { CATEGORIES, HIGH_SIGNAL_CATEGORIES, type Category } from './categories'
import type { FeedFilters, InsightType, WindowKey } from '../services/types'

export type FeedView = 'stream' | 'video'
export interface FeedScope extends FeedFilters {
  range: WindowKey
  view: FeedView
}

export const DEFAULT_FEED_FILTERS = (): FeedFilters => ({
  channels: [], categories: [...HIGH_SIGNAL_CATEGORIES], insight_type: null,
  date_from: null, date_to: null, search: '', include_toxic: false, sort: 'newest',
})
export const DEFAULT_SAVED_FILTERS = (): FeedFilters => ({
  ...DEFAULT_FEED_FILTERS(), categories: [], include_toxic: false, sort: 'newest',
})

const insightTypes: InsightType[] = ['video_idea', 'editing_tip', 'feature_request', 'collaboration_ask', 'improvement_suggestion', 'content_format_request', 'other']
const value = (v: unknown): string | null => typeof v === 'string' ? v : Array.isArray(v) && typeof v[0] === 'string' ? v[0] : null

export function serializeFeedScope(scope: FeedScope): Record<string, string> {
  const q: Record<string, string> = {}
  if (scope.channels.length) q.channels = scope.channels.join(',')
  if (scope.categories_specified || scope.categories.join(',') !== HIGH_SIGNAL_CATEGORIES.join(',')) q.categories = scope.categories.join(',')
  if (scope.intent && scope.intent !== 'all') q.intent = scope.intent
  if (scope.insight_type) q.insight_type = scope.insight_type
  if (scope.date_from) q.date_from = scope.date_from
  if (scope.date_to) q.date_to = scope.date_to
  if (scope.search) q.search = scope.search
  if (scope.include_toxic) q.include_toxic = '1'
  if (scope.sort !== 'newest') q.sort = scope.sort
  if (scope.range !== '7d') q.range = scope.range
  if (scope.view !== 'stream') q.view = scope.view
  return q
}

export function parseFeedScope(query: Record<string, unknown>, base: FeedFilters = DEFAULT_FEED_FILTERS()): FeedScope {
  const next: FeedScope = { ...base, range: '7d', view: 'stream' }
  const channels = value(query.channels)
  if (channels !== null) next.channels = channels.split(',').filter(Boolean)
  const categories = value(query.categories)
  if (categories !== null) {
    next.categories = categories.split(',').filter((c): c is Category => (CATEGORIES as readonly string[]).includes(c))
    next.categories_specified = true
  }
  const intent = value(query.intent)
  if (intent === 'all' || intent === 'ideas' || intent === 'actionable') next.intent = intent
  const insight = value(query.insight_type)
  if (insight && insightTypes.includes(insight as InsightType)) next.insight_type = insight as InsightType
  for (const key of ['date_from', 'date_to', 'search'] as const) {
    const item = value(query[key]); if (item !== null) next[key] = item
  }
  next.include_toxic = ['1', 'true'].includes(value(query.include_toxic) ?? '')
  const sort = value(query.sort); if (sort === 'newest' || sort === 'oldest') next.sort = sort
  const range = value(query.range); if (range === '7d' || range === '30d' || range === '90d' || range === 'custom' || range === 'all') next.range = range
  if (value(query.view) === 'video') next.view = 'video'
  return next
}
