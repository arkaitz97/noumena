// Central registry for the 8-value LLM category enum from `llm-processing`.
// Color tokens are Tailwind class strings (background + text + ring) so
// callers can simply spread them onto a pill element.

export const CATEGORIES = [
  'question',
  'content_idea',
  'collaboration_ask',
  'praise',
  'constructive_criticism',
  'toxic_criticism',
  'spam',
  'other',
] as const

export type Category = typeof CATEGORIES[number]

/** Categories surfaced by default in the feed (the high-signal seven). */
export const HIGH_SIGNAL_CATEGORIES: Category[] = [
  'question',
  'content_idea',
  'collaboration_ask',
  'praise',
  'constructive_criticism',
  'other',
]

/** Categories gated behind the `include_toxic` toggle. */
export const TOXIC_CATEGORIES: Category[] = ['toxic_criticism', 'spam']

export const CATEGORY_LABELS: Record<Category, string> = {
  question: 'Question',
  content_idea: 'Content idea',
  collaboration_ask: 'Collaboration',
  praise: 'Praise',
  constructive_criticism: 'Constructive criticism',
  toxic_criticism: 'Toxic',
  spam: 'Spam',
  other: 'Other',
}

/**
 * Pill colors per category. Each entry is a triplet of Tailwind classes
 * (`bg-* text-* ring-*`) ready to be applied to a pill / chip element.
 */
export const CATEGORY_COLORS: Record<Category, string> = {
  question: 'bg-brand-50 text-brand-700 ring-brand-700/10',
  content_idea: 'bg-brand-50 text-brand-700 ring-brand-700/10',
  collaboration_ask: 'bg-sky-50 text-sky-700 ring-sky-700/10',
  praise: 'bg-emerald-50 text-emerald-700 ring-emerald-700/10',
  constructive_criticism: 'bg-amber-50 text-amber-700 ring-amber-700/10', // unslop-ignore
  toxic_criticism: 'bg-rose-50 text-rose-700 ring-rose-700/10',
  spam: 'bg-zinc-100 text-zinc-600 ring-zinc-500/10',
  other: 'bg-zinc-50 text-zinc-700 ring-zinc-500/10',
}

export function categoryLabel(c: string | null | undefined): string {
  if (!c) return 'Uncategorized'
  return (CATEGORY_LABELS as Record<string, string>)[c] ?? c
}

export function categoryColor(c: string | null | undefined): string {
  if (!c) return CATEGORY_COLORS.other
  return (CATEGORY_COLORS as Record<string, string>)[c] ?? CATEGORY_COLORS.other
}
