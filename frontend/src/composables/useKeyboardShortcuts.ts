import { onBeforeUnmount, onMounted, ref } from 'vue'

export interface UseKeyboardShortcutsOptions {
  onStar?: () => void
  onNavigateUp?: () => void
  onNavigateDown?: () => void
  onFocusSearch?: () => void
  onToggleHelp?: () => void
}

export function useKeyboardShortcuts(opts: UseKeyboardShortcutsOptions = {}) {
  const helpVisible = ref(false)

  function handler(e: KeyboardEvent) {
    const tag = (e.target as HTMLElement).tagName.toLowerCase()
    const isInput = tag === 'input' || tag === 'textarea' || tag === 'select' || (e.target as HTMLElement).isContentEditable

    if (e.key === '?' && !isInput) {
      e.preventDefault()
      helpVisible.value = !helpVisible.value
      opts.onToggleHelp?.()
      return
    }

    if (helpVisible.value) return

    if (e.key === '/' && !isInput) {
      e.preventDefault()
      opts.onFocusSearch?.()
      return
    }

    if (isInput) return

    switch (e.key) {
      case 's':
        e.preventDefault()
        opts.onStar?.()
        break
      case 'j':
        e.preventDefault()
        opts.onNavigateDown?.()
        break
      case 'k':
        e.preventDefault()
        opts.onNavigateUp?.()
        break
    }
  }

  onMounted(() => {
    window.addEventListener('keydown', handler)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('keydown', handler)
  })

  return { helpVisible }
}
