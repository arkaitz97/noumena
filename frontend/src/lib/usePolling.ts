// Visibility-gated polling composable.
//
// Single-flights overlapping calls via a per-instance `inFlight` promise;
// pauses while `document.visibilityState !== 'visible'`; tears down in
// `onBeforeUnmount` so view navigation never leaks intervals.

import { onBeforeUnmount } from 'vue'

export interface UsePollingOptions {
  /** Default true. When false the poll fires regardless of tab visibility. */
  onlyWhenVisible?: boolean
  /** When true, fire `fn` once immediately on registration. Default false. */
  immediate?: boolean
}

export interface PollingHandle {
  stop: () => void
  /** Trigger an immediate poll out-of-band (still single-flighted). */
  trigger: () => Promise<void>
}

export function usePolling(
  fn: () => Promise<unknown>,
  intervalMs: number,
  opts: UsePollingOptions = {},
): PollingHandle {
  const onlyWhenVisible = opts.onlyWhenVisible ?? true
  let timer: number | null = null
  let inFlight: Promise<void> | null = null
  let stopped = false

  function visible(): boolean {
    if (!onlyWhenVisible) return true
    return typeof document === 'undefined' || document.visibilityState === 'visible'
  }

  async function tick(): Promise<void> {
    if (stopped) return
    if (!visible()) return
    if (inFlight) return // single-flight: skip overlapping ticks.
    inFlight = (async () => {
      try {
        await fn()
      } catch {
        // Swallow — the wrapped fn is responsible for surfacing its own
        // toasts. Polling continues regardless.
      } finally {
        inFlight = null
      }
    })()
    await inFlight
  }

  function start(): void {
    if (timer !== null) return
    timer = window.setInterval(() => {
      void tick()
    }, intervalMs)
  }

  function pause(): void {
    if (timer !== null) {
      window.clearInterval(timer)
      timer = null
    }
  }

  function onVisibilityChange(): void {
    if (stopped) return
    if (visible()) {
      start()
      void tick()
    } else {
      pause()
    }
  }

  if (typeof document !== 'undefined' && onlyWhenVisible) {
    document.addEventListener('visibilitychange', onVisibilityChange)
  }

  if (opts.immediate) {
    void tick()
  }
  if (visible()) start()

  function stop(): void {
    stopped = true
    pause()
    if (typeof document !== 'undefined' && onlyWhenVisible) {
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  }

  // Auto-teardown — usePolling is only valid inside a setup() context.
  onBeforeUnmount(stop)

  return {
    stop,
    trigger: () => tick(),
  }
}
