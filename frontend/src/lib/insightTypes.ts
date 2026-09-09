// Single registry for actionable-insight types: the enum order used by the
// feed filter, the human labels, and the pill color tokens. Duplicated
// inline maps in CommentCard / FilterBar / WeeklyDigestCard used to drift.

import type { InsightType } from '../services/types'

export const INSIGHT_TYPES: InsightType[] = [
  'video_idea',
  'editing_tip',
  'feature_request',
  'collaboration_ask',
  'improvement_suggestion',
  'content_format_request',
  'other',
]

export const INSIGHT_TYPE_LABELS: Record<InsightType, string> = {
  video_idea: 'Video Ideas',
  editing_tip: 'Editing Tips',
  feature_request: 'Feature Requests',
  collaboration_ask: 'Collaboration Asks',
  improvement_suggestion: 'Improvements',
  content_format_request: 'Format Requests',
  other: 'Other',
}

/** Pill color classes for a single insight chip (bg + text + ring). */
export const INSIGHT_TYPE_COLORS: Record<InsightType, string> = {
  video_idea: 'bg-blue-50 text-blue-700 ring-blue-600/20 dark:bg-blue-950/40 dark:text-blue-300 dark:ring-blue-500/30',
  editing_tip: 'bg-amber-50 text-amber-700 ring-amber-600/20 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-500/30', // unslop-ignore
  feature_request: 'bg-brand-50 text-brand-700 ring-brand-600/20 dark:bg-brand-950/40 dark:text-brand-300 dark:ring-brand-500/30',
  collaboration_ask: 'bg-green-50 text-green-700 ring-green-600/20 dark:bg-green-950/40 dark:text-green-300 dark:ring-green-500/30',
  improvement_suggestion: 'bg-orange-50 text-orange-700 ring-orange-600/20 dark:bg-orange-950/40 dark:text-orange-300 dark:ring-orange-500/30', // unslop-ignore
  content_format_request: 'bg-pink-50 text-pink-700 ring-pink-600/20 dark:bg-pink-950/40 dark:text-pink-300 dark:ring-pink-500/30',
  other: 'bg-zinc-50 text-zinc-700 ring-zinc-600/20 dark:bg-zinc-800 dark:text-zinc-300 dark:ring-zinc-500/30',
}

export function insightTypeLabel(t: InsightType | string): string {
  return (INSIGHT_TYPE_LABELS as Record<string, string>)[t] ?? t
}

export function insightTypeColor(t: InsightType | string): string {
  return (INSIGHT_TYPE_COLORS as Record<string, string>)[t] ?? INSIGHT_TYPE_COLORS.other
}
