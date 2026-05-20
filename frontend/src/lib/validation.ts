// Password policy shared by Signup and ResetPassword views.
// Mirrors backend rules in internal/auth: min 8 chars, ≥1 uppercase, ≥1 digit.

export interface PasswordRule {
  id: string
  label: string
  test: (value: string) => boolean
}

export const passwordRules: PasswordRule[] = [
  { id: 'length', label: 'At least 8 characters', test: (v) => v.length >= 8 },
  { id: 'upper', label: 'One uppercase letter', test: (v) => /[A-Z]/.test(v) },
  { id: 'digit', label: 'One number', test: (v) => /[0-9]/.test(v) },
]

export interface PasswordValidation {
  valid: boolean
  /** 0..passwordRules.length — how many rules are satisfied. */
  score: number
  /** rule.id → boolean */
  results: Record<string, boolean>
  /** First failing rule label, or null when valid. */
  firstError: string | null
}

export function validatePassword(value: string): PasswordValidation {
  const results: Record<string, boolean> = {}
  let score = 0
  let firstError: string | null = null
  for (const rule of passwordRules) {
    const ok = rule.test(value)
    results[rule.id] = ok
    if (ok) score++
    else if (firstError === null) firstError = rule.label
  }
  return { valid: score === passwordRules.length, score, results, firstError }
}
