// Relative-time formatting using `Intl.RelativeTimeFormat`. Replaces a
// `date-fns` dependency for the small set of "Xh ago" / "in 3d" strings the
// dashboard renders.

const RTF = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })

const DIVISIONS: Array<{ amount: number; unit: Intl.RelativeTimeFormatUnit }> = [
  { amount: 60, unit: 'second' },
  { amount: 60, unit: 'minute' },
  { amount: 24, unit: 'hour' },
  { amount: 7, unit: 'day' },
  { amount: 4.34524, unit: 'week' },
  { amount: 12, unit: 'month' },
  { amount: Number.POSITIVE_INFINITY, unit: 'year' },
]

export function relative(date: string | Date | null | undefined): string {
  if (!date) return ''
  const d = typeof date === 'string' ? new Date(date) : date
  if (Number.isNaN(d.getTime())) return ''
  let diff = (d.getTime() - Date.now()) / 1000
  for (const div of DIVISIONS) {
    if (Math.abs(diff) < div.amount) {
      return RTF.format(Math.round(diff), div.unit)
    }
    diff /= div.amount
  }
  return ''
}

/** Short absolute date (e.g. "Apr 24, 2026"). */
export function shortDate(date: string | Date | null | undefined): string {
  if (!date) return ''
  const d = typeof date === 'string' ? new Date(date) : date
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleDateString('en', { year: 'numeric', month: 'short', day: 'numeric' })
}
