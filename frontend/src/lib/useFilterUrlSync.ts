// Two-way (URL ↔ store) filter-state sync composable used by both the
// insights feed and the saved-comments view. Each call site has its own
// URL, so the two views never accidentally share filter values.

import { onBeforeUnmount, watch } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'

export interface FilterUrlSyncBinding<T extends object> {
  /** Read the current filter snapshot from the store. */
  get: () => T
  /** Apply a partial filter snapshot parsed from the URL to the store. */
  apply: (partial: Partial<T>) => void
  /** Serialise the filter snapshot to a URL query record. */
  serialise: (filters: T) => LocationQueryRaw
  /** Parse a URL query record into a partial snapshot. */
  parse: (query: Record<string, string | (string | null)[] | null>) => Partial<T>
}

export function useFilterUrlSync<T extends object>(
  binding: FilterUrlSyncBinding<T>,
): void {
  const route = useRoute()
  const router = useRouter()
  let applyingFromUrl = false
  let writeTimeout: number | null = null

  // 1) On mount: hydrate from URL once.
  applyingFromUrl = true
  try {
    const parsed = binding.parse(route.query as Record<string, string | (string | null)[] | null>)
    if (Object.keys(parsed).length > 0) {
      binding.apply(parsed)
    }
  } finally {
    // Release the flag on the next microtask so the immediate watch trigger
    // (caused by apply()) is suppressed.
    queueMicrotask(() => {
      applyingFromUrl = false
    })
  }

  // 2) Watch store filters → push to URL (debounced).
  const stop = watch(
    () => binding.get(),
    (next) => {
      if (applyingFromUrl) return
      if (writeTimeout !== null) window.clearTimeout(writeTimeout)
      writeTimeout = window.setTimeout(() => {
        const query = binding.serialise(next)
        const same = JSON.stringify(query) === JSON.stringify(route.query)
        if (!same) {
          void router.replace({ path: route.path, query })
        }
      }, 200)
    },
    { deep: true },
  )

  onBeforeUnmount(() => {
    stop()
    if (writeTimeout !== null) window.clearTimeout(writeTimeout)
  })
}
