// Tiny reactive toast utility. Mounted via <ToastHost /> in App.vue.
import { reactive } from 'vue'

export type ToastVariant = 'success' | 'error' | 'info'

export interface Toast {
  id: number
  variant: ToastVariant
  message: string
}

interface ToastState {
  items: Toast[]
}

const state: ToastState = reactive({ items: [] })
let nextId = 1

function push(variant: ToastVariant, message: string, durationMs = 4000): number {
  const id = nextId++
  state.items.push({ id, variant, message })
  if (durationMs > 0) {
    window.setTimeout(() => dismiss(id), durationMs)
  }
  return id
}

export function dismiss(id: number): void {
  const idx = state.items.findIndex((t) => t.id === id)
  if (idx >= 0) state.items.splice(idx, 1)
}

export const toast = {
  success: (msg: string, durationMs?: number) => push('success', msg, durationMs),
  error: (msg: string, durationMs?: number) => push('error', msg, durationMs),
  info: (msg: string, durationMs?: number) => push('info', msg, durationMs),
}

export function useToastState(): ToastState {
  return state
}
