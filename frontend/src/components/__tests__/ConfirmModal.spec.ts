// ConfirmModal.spec.ts — confirm/cancel emits, ESC behaviour, destructive styling, slot rendering.
//
// NOTE: ConfirmModal's actual API uses `description` (not `body`),
// `destructive: boolean` (not `variant`), and exposes default + `#footer`
// slots with a `customFooter: boolean` escape hatch (consumed by AccountView's
// two-step deletion). Tests assert the active component contract.
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ConfirmModal from '../ConfirmModal.vue'

describe('ConfirmModal', () => {
  it('emits confirm when the confirm button is clicked', async () => {
    const wrapper = mount(ConfirmModal, {
      props: { open: true, title: 'Are you sure?', description: 'This cannot be undone.' },
      attachTo: document.body,
    })
    const buttons = Array.from(document.querySelectorAll<HTMLButtonElement>('button'))
    const confirmBtn = buttons.find((b) => b.textContent?.trim() === 'Confirm')
    expect(confirmBtn).toBeTruthy()
    confirmBtn!.click()
    expect(wrapper.emitted('confirm')).toBeTruthy()
    wrapper.unmount()
  })

  it('emits cancel when ESC is pressed', async () => {
    const wrapper = mount(ConfirmModal, {
      props: { open: true, title: 'Are you sure?' },
      attachTo: document.body,
    })
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('cancel')).toBeTruthy()
    expect(wrapper.emitted('confirm')).toBeFalsy()
    wrapper.unmount()
  })

  it('uses the destructive (rose) confirm button style when destructive=true', () => {
    const wrapper = mount(ConfirmModal, {
      props: { open: true, title: 'Delete?', destructive: true },
      attachTo: document.body,
    })
    const html = document.body.innerHTML
    expect(html).toMatch(/bg-rose-600/)
    wrapper.unmount()
  })

  it('renders custom slot content (used by AccountView two-step deletion)', () => {
    const wrapper = mount(ConfirmModal, {
      props: { open: true, title: 'Step', customFooter: true },
      slots: { default: '<p data-test="step-content">Type your email to confirm</p>' },
      attachTo: document.body,
    })
    const stepEl = document.querySelector('[data-test="step-content"]')
    expect(stepEl).not.toBeNull()
    expect(stepEl!.textContent).toContain('Type your email to confirm')
    wrapper.unmount()
  })
})
