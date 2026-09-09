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
    actionable_insights: null,
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

  it('shows insights count badge when comment has actionable_insights', () => {
    const wrapper = mount(CommentCard, {
      props: {
        comment: makeComment({
          actionable_insights: [
            { type: 'video_idea', description: 'Make a tutorial', confidence: 0.85 },
          ],
        }),
        isStarred: false,
      },
    })
    expect(wrapper.text()).toContain('1 insight')
  })

  it('links the channel name and video title to youtube', () => {
    const wrapper = mount(CommentCard, {
      props: {
        comment: makeComment({
          platform_channel_id: 'UC123',
          video_id: 'v456',
          video_title: 'My video',
        }),
        isStarred: false,
      },
    })
    const channel = wrapper.get('a[href="https://www.youtube.com/channel/UC123"]')
    expect(channel.text()).toContain('Test channel')
    const video = wrapper.get('a[href="https://www.youtube.com/watch?v=v456"]')
    expect(video.text()).toContain('My video')
  })

  it('omits the video link when video context is missing', () => {
    const wrapper = mount(CommentCard, {
      props: { comment: makeComment({ video_id: null, video_title: null }), isStarred: false },
    })
    expect(wrapper.find('a[href^="https://www.youtube.com/watch"]').exists()).toBe(false)
  })
})
