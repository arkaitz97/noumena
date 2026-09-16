import { HIGH_SIGNAL_CATEGORIES, type Category } from './categories'
import type { FeedFilters, InsightType } from '../services/types'

export type FeedIntent = 'all' | 'actionable' | 'questions' | 'ideas' | 'critique'

export interface IntentPreset {
  id: FeedIntent
  label: string
  categories: Category[]
  insight_type: InsightType | null
}

export const INTENT_PRESETS: IntentPreset[] = [
  { id: 'all', label: 'All', categories: [...HIGH_SIGNAL_CATEGORIES], insight_type: null },
  { id: 'actionable', label: 'Actionable', categories: ['question', 'content_idea', 'constructive_criticism'], insight_type: null },
  { id: 'questions', label: 'Questions', categories: ['question'], insight_type: null },
  { id: 'ideas', label: 'Ideas', categories: ['content_idea'], insight_type: 'video_idea' },
  { id: 'critique', label: 'Critique', categories: ['constructive_criticism'], insight_type: null },
]

export function presetFilters(intent: FeedIntent): Pick<FeedFilters, 'categories' | 'insight_type'> {
  const preset = INTENT_PRESETS.find((item) => item.id === intent) ?? INTENT_PRESETS[0]
  return { categories: [...preset.categories], insight_type: preset.insight_type }
}
