// CommentCard.spec.ts — colocated component test for CommentCard.vue.
//
// Contract under test (per `openspec/changes/frontend-component-extraction/specs/frontend-component-library/spec.md`):
//   - typed props `comment: Comment`, `isStarred?: boolean`
//   - emits `toggle-star` with the comment id when the star button is activated
//   - shows a "Processing…" badge when `comment.processed_at === null`
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import CommentCard from '../CommentCard.vue'
import type { Comment } from '../../services/types'

function makeComment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 'abc',
    channel_id: 'ch_1',
    channel_display_name: 'Test channel',
    platform_comment_id: 'p1',
    author_handle: '@alice',
    author_display_name: 'Alice',
    body: 'hello world',
    posted_at: new Date().toISOString(),
    processed_at: new Date().toISOString(),
    category: 'idea',
    topic: null,
    sentiment_score: 0.5,
    is_starred: false,
    starred_at: null,
    ...overrides,
  }
}

describe('CommentCard', () => {
  it('renders the comment body and author', () => {
    const wrapper = mount(CommentCard, {
      props: { comment: makeComment(), isStarred: false },
    })
    expect(wrapper.text()).toContain('hello world')
    expect(wrapper.text()).toContain('@alice')
  })

  it('shows the Processing… badge when processed_at is null', () => {
    const wrapper = mount(CommentCard, {
      props: { comment: makeComment({ processed_at: null }), isStarred: false },
    })
    expect(wrapper.text()).toMatch(/Processing/i)
  })

  it('emits toggle-star with the comment id when the star button is clicked', async () => {
    const wrapper = mount(CommentCard, {
      props: { comment: makeComment({ id: 'abc' }), isStarred: false },
    })
    await wrapper.get('button[aria-label="Star comment"]').trigger('click')
    const events = wrapper.emitted('toggle-star')
    expect(events).toBeTruthy()
    expect(events![0][0]).toBe('abc')
  })
})
