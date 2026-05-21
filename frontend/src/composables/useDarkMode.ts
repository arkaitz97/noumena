import { computed, onMounted, onUnmounted, ref } from 'vue'

const STORAGE_KEY = 'noumena-dark-mode'
const MEDIA_QUERY = '(prefers-color-scheme: dark)'

function getSystemPreference(): boolean {
  return window.matchMedia(MEDIA_QUERY).matches
}

function getStoredPreference(): boolean | null {
  const v = localStorage.getItem(STORAGE_KEY)
  if (v === null) return null
  return v === 'true'
}

function applyDark(dark: boolean): void {
  document.documentElement.classList.toggle('dark', dark)
}

export function useDarkMode() {
  const stored = getStoredPreference()
  const isDark = ref(stored !== null ? stored : getSystemPreference())

  let mediaQuery: MediaQueryList | null = null

  function onSystemChange(e: MediaQueryListEvent): void {
    if (getStoredPreference() === null) {
      isDark.value = e.matches
      applyDark(e.matches)
    }
  }

  onMounted(() => {
    applyDark(isDark.value)
    mediaQuery = window.matchMedia(MEDIA_QUERY)
    mediaQuery.addEventListener('change', onSystemChange)
  })

  onUnmounted(() => {
    if (mediaQuery) {
      mediaQuery.removeEventListener('change', onSystemChange)
    }
  })

  function toggleDarkMode(): void {
    isDark.value = !isDark.value
    localStorage.setItem(STORAGE_KEY, String(isDark.value))
    applyDark(isDark.value)
  }

  return { isDark: computed(() => isDark.value), toggleDarkMode }
}
