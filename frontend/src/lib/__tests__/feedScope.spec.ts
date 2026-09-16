import { describe, expect, it } from 'vitest'
import { DEFAULT_FEED_FILTERS, DEFAULT_SAVED_FILTERS, parseFeedScope, serializeFeedScope } from '../feedScope'

describe('FeedScope', () => {
  it('round-trips explicit empty categories', () => {
    const scope = { ...DEFAULT_FEED_FILTERS(), categories: [], categories_specified: true, range: '7d' as const, view: 'stream' as const }
    expect(parseFeedScope(serializeFeedScope(scope)).categories).toEqual([])
    expect(serializeFeedScope(scope).categories).toBe('')
  })

  it('hydrates omitted values from view defaults', () => {
    const parsed = parseFeedScope({}, DEFAULT_SAVED_FILTERS())
    expect(parsed.search).toBe('')
    expect(parsed.intent).toBeUndefined()
    expect(parsed.range).toBe('7d')
  })
})
