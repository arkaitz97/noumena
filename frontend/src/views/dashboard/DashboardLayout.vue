<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import {
  LayoutDashboard,
  Bookmark,
  Tv,
  CreditCard,
  UserCircle,
  Settings,
  LogOut,
  Users,
  ChevronDown,
  Sun,
  Moon,
  Menu,
  X,
} from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'
import { useDarkMode } from '../../composables/useDarkMode'
import { useWorkspaceRole } from '../../composables/useWorkspaceRole'
import { useRateLimitStore } from '../../stores/rateLimit'
import { toast } from '../../lib/toast'
import WorkspaceSwitcher from '../../components/WorkspaceSwitcher.vue'
import RateLimitBanner from '../../components/RateLimitBanner.vue'
import TokenExpiryBanner from '../../components/TokenExpiryBanner.vue'
import { usePolling } from '../../lib/usePolling'
import { getTokenExpiry } from '../../services/tokenExpiry'
import type { TokenExpiryChannel } from '../../services/tokenExpiry'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { isDark, toggleDarkMode } = useDarkMode()
const { isAdmin } = useWorkspaceRole()
const rateLimit = useRateLimitStore()

const navItems = [
  { to: '/dashboard', label: 'Insights Feed', icon: LayoutDashboard, exact: true },
  { to: '/dashboard/saved', label: 'Saved Comments', icon: Bookmark },
  { to: '/settings/channels', label: 'Channels', icon: Tv },
  { to: '/settings/billing', label: 'Billing', icon: CreditCard },
  { to: '/settings/account', label: 'Account', icon: UserCircle },
]

function isActive(to: string, exact: boolean | undefined): boolean {
  if (exact) return route.path === to
  return route.path === to || route.path.startsWith(to + '/')
}

// User menu
const menuOpen = ref(false)
const menuRef = ref<HTMLElement | null>(null)

function toggleMenu() {
  menuOpen.value = !menuOpen.value
}

function closeMenuOnOutside(event: MouseEvent) {
  if (!menuRef.value) return
  if (!menuRef.value.contains(event.target as Node)) menuOpen.value = false
}

document.addEventListener('click', closeMenuOnOutside)
onBeforeUnmount(() => document.removeEventListener('click', closeMenuOnOutside))

const userInitial = computed(() => (auth.user?.email || '?').slice(0, 1).toUpperCase())

// Mobile navigation drawer (< lg)
const drawerOpen = ref(false)
const drawerRef = ref<HTMLElement | null>(null)
const hamburgerRef = ref<HTMLButtonElement | null>(null)

function openDrawer(): void {
  drawerOpen.value = true
}

function closeDrawer(): void {
  drawerOpen.value = false
}

function onDrawerKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') {
    e.preventDefault()
    closeDrawer()
  }
}

watch(drawerOpen, async (open) => {
  if (open) {
    await nextTick()
    drawerRef.value?.querySelector<HTMLElement>('a, button')?.focus()
  } else {
    hamburgerRef.value?.focus()
  }
})

async function handleLogout() {
  menuOpen.value = false
  drawerOpen.value = false
  try {
    await auth.logout()
    router.push('/login')
  } catch {
    // Server did not confirm logout — the HttpOnly refresh cookie is still
    // valid, so clearing local state would fake a logged-out user that
    // re-authenticates on next load. Keep the session, tell the user.
    toast.error('Could not log out. Please try again.')
  }
}

async function handleLogoutAll() {
  menuOpen.value = false
  drawerOpen.value = false
  try {
    await auth.logoutAll()
    router.push('/login')
  } catch {
    toast.error('Could not log out of all devices. Please try again.')
  }
}

const menuTriggerRef = ref<HTMLButtonElement | null>(null)

function menuItems(): HTMLElement[] {
  if (!menuRef.value) return []
  return Array.from(menuRef.value.querySelectorAll<HTMLElement>('[role="menuitem"]'))
}

function focusMenuItemAt(index: number): void {
  const items = menuItems()
  if (items.length === 0) return
  const next = (index + items.length) % items.length
  items[next].focus()
}

function onMenuKeydown(e: KeyboardEvent): void {
  if (!menuOpen.value) return
  const items = menuItems()
  if (items.length === 0) return
  const idx = items.indexOf(document.activeElement as HTMLElement)
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    focusMenuItemAt(idx + 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    focusMenuItemAt(idx - 1)
  } else if (e.key === 'Home') {
    e.preventDefault()
    focusMenuItemAt(0)
  } else if (e.key === 'End') {
    e.preventDefault()
    focusMenuItemAt(items.length - 1)
  } else if (e.key === 'Escape') {
    e.preventDefault()
    menuOpen.value = false
    menuTriggerRef.value?.focus()
  }
}

watch(menuOpen, async (open) => {
  if (open) {
    await nextTick()
    focusMenuItemAt(0)
  }
})

// Rate limit state
function dismissRateLimit() {
  rateLimit.dismiss()
}

// Token-expiry banner state. Fetched on mount (the layout sits behind the
// auth guard, so mount ≈ login landing), on a visibility-gated 60s heartbeat
// (same cadence as InsightsFeed's feed poll), and whenever the user lands on
// /settings/channels — the reconnect flow lives there, and a successful
// reauthorize should clear the banner promptly.
const expiringChannels = ref<TokenExpiryChannel[]>([])
const tokenExpiryDismissed = ref(false)

async function refreshTokenExpiry(): Promise<void> {
  if (tokenExpiryDismissed.value) return
  try {
    const res = await getTokenExpiry()
    expiringChannels.value = res?.channels ?? []
  } catch {
    // Non-fatal — keep the last known state; the next heartbeat retries.
  }
}

const tokenExpiryPoll = usePolling(refreshTokenExpiry, 60_000)

onMounted(() => {
  void tokenExpiryPoll.trigger()
})

watch(
  () => route.path,
  (path) => {
    if (path.startsWith('/settings/channels')) void tokenExpiryPoll.trigger()
  },
)

function dismissTokenExpiry() {
  tokenExpiryDismissed.value = true
  expiringChannels.value = []
}
</script>

<template>
  <div class="min-h-screen bg-zinc-50 dark:bg-zinc-950 flex">
    <!-- Sidebar -->
    <aside class="hidden lg:flex flex-col w-64 bg-white dark:bg-zinc-900 border-r border-zinc-200 dark:border-zinc-800 sticky top-0 h-screen">
      <div class="px-6 py-5 border-b border-zinc-100 dark:border-zinc-800">
        <RouterLink to="/dashboard" class="flex items-center gap-2 group">
          <div class="w-8 h-8 rounded-lg bg-zinc-900 dark:bg-zinc-100 flex items-center justify-center shadow-md">
            <svg class="w-4 h-4 text-white dark:text-zinc-900" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
          </div>
          <span class="font-bold text-lg tracking-tight text-zinc-900 dark:text-zinc-100">Noumena</span>
        </RouterLink>
      </div>

      <nav class="flex-1 px-3 py-4 space-y-1">
        <RouterLink
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          :class="[
            isActive(item.to, item.exact)
              ? 'bg-zinc-100 dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100'
              : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-50 dark:hover:bg-zinc-800/50',
            'flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors',
          ]"
        >
          <component :is="item.icon" class="w-4 h-4" />
          {{ item.label }}
        </RouterLink>
      </nav>

      <div class="px-6 py-4 border-t border-zinc-100 dark:border-zinc-800 text-xs text-zinc-500 dark:text-zinc-400">
        © {{ new Date().getFullYear() }} Noumena
      </div>
    </aside>

    <!-- Main column -->
    <div class="flex-1 flex flex-col min-w-0">
      <!-- Top bar -->
      <header class="bg-white/80 dark:bg-zinc-900/80 backdrop-blur-md border-b border-zinc-200 dark:border-zinc-800 sticky top-0 z-30">
        <div class="px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between gap-4">
          <!-- Mobile brand + hamburger -->
          <div class="lg:hidden flex items-center gap-2 min-w-0">
            <button
              ref="hamburgerRef"
              type="button"
              aria-label="Open navigation menu"
              :aria-expanded="drawerOpen"
              aria-controls="mobile-nav-drawer"
              @click="openDrawer"
              class="flex items-center justify-center h-11 w-11 rounded-lg text-zinc-600 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors"
            >
              <Menu class="w-5 h-5" />
            </button>
            <RouterLink to="/dashboard" class="flex items-center gap-2 min-w-0">
              <div class="w-7 h-7 rounded-lg bg-zinc-900 dark:bg-zinc-100 flex items-center justify-center">
                <svg class="w-4 h-4 text-white dark:text-zinc-900" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
                </svg>
              </div>
              <span class="font-bold tracking-tight text-zinc-900 dark:text-zinc-100 truncate">Noumena</span>
            </RouterLink>
          </div>

          <div class="hidden lg:flex items-center gap-3 flex-1">
            <WorkspaceSwitcher />
          </div>

          <div class="flex items-center gap-2">
            <button
              @click="toggleDarkMode"
              class="flex items-center justify-center h-11 w-11 rounded-lg text-zinc-500 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors"
              :aria-label="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
            >
              <Sun v-if="isDark" class="w-4 h-4" />
              <Moon v-else class="w-4 h-4" />
            </button>

            <div ref="menuRef" class="relative" @keydown="onMenuKeydown">
              <button
                ref="menuTriggerRef"
                @click.stop="toggleMenu"
                class="flex items-center gap-2 pl-2.5 pr-3 py-1.5 min-h-11 rounded-full border border-zinc-200 dark:border-zinc-700 hover:border-zinc-300 dark:hover:border-zinc-600 hover:bg-zinc-50 dark:hover:bg-zinc-800 transition-colors"
                :aria-expanded="menuOpen"
                aria-haspopup="menu"
              >
                <span class="h-7 w-7 rounded-full bg-brand-600 text-white font-semibold text-xs flex items-center justify-center">
                  {{ userInitial }}
                </span>
                <span class="hidden sm:inline text-sm font-medium text-zinc-700 dark:text-zinc-300 max-w-[12rem] truncate">
                  {{ auth.user?.email || 'Account' }}
                </span>
                <ChevronDown class="w-4 h-4 text-zinc-500 dark:text-zinc-400" />
              </button>

              <transition name="menu">
                <div
                  v-if="menuOpen"
                  role="menu"
                  aria-label="User menu"
                  class="absolute right-0 mt-2 w-64 bg-white dark:bg-zinc-900 shadow-lg ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl py-2 origin-top-right"
                >
                  <div class="px-4 py-3 border-b border-zinc-100 dark:border-zinc-800">
                    <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400">Signed in as</p>
                    <p class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 truncate">{{ auth.user?.email || '—' }}</p>
                  </div>
                  <div class="py-1">
                    <RouterLink
                      to="/settings/account"
                      role="menuitem"
                      @click="menuOpen = false"
                      class="flex items-center gap-2 px-4 py-2 text-sm text-zinc-700 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-800"
                    >
                      <Settings class="w-4 h-4" /> Settings
                    </RouterLink>
                    <RouterLink
                      v-if="isAdmin"
                      to="/settings/members"
                      role="menuitem"
                      @click="menuOpen = false"
                      class="flex items-center gap-2 px-4 py-2 text-sm text-zinc-700 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-800"
                    >
                      <Users class="w-4 h-4" /> Manage members
                    </RouterLink>
                    <button
                      role="menuitem"
                      @click="handleLogout"
                      class="w-full text-left flex items-center gap-2 px-4 py-2 text-sm text-zinc-700 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-800"
                    >
                      <LogOut class="w-4 h-4" /> Log out
                    </button>
                    <button
                      role="menuitem"
                      @click="handleLogoutAll"
                      class="w-full text-left flex items-center gap-2 px-4 py-2 text-sm text-rose-600 hover:bg-rose-50 dark:hover:bg-rose-950/30"
                    >
                      <Users class="w-4 h-4" /> Log out of all devices
                    </button>
                  </div>
                </div>
              </transition>
            </div>
          </div>
        </div>

        <!-- Notifications -->
        <div class="px-4 sm:px-6 lg:px-8 pb-3 space-y-2">
          <RateLimitBanner
            :visible="rateLimit.visible"
            :retry-after-seconds="rateLimit.retryAfterSeconds"
            @dismiss="dismissRateLimit"
          />
          <TokenExpiryBanner
            :visible="!tokenExpiryDismissed && expiringChannels.length > 0"
            :channels="expiringChannels"
            @dismiss="dismissTokenExpiry"
          />
        </div>
      </header>

      <main class="flex-1">
        <RouterView />
      </main>
    </div>

    <!-- Mobile navigation drawer (< lg) -->
    <transition name="drawer-fade">
      <div
        v-if="drawerOpen"
        class="fixed inset-0 z-40 lg:hidden"
        aria-hidden="true"
        @click="closeDrawer"
      ></div>
    </transition>

    <transition name="drawer-slide">
      <aside
        v-if="drawerOpen"
        id="mobile-nav-drawer"
        ref="drawerRef"
        class="fixed left-0 top-0 bottom-0 z-40 w-72 max-w-[85vw] lg:hidden bg-white dark:bg-zinc-900 border-r border-zinc-200 dark:border-zinc-800 flex flex-col shadow-2xl"
        role="dialog"
        aria-modal="true"
        aria-label="Navigation"
        @keydown="onDrawerKeydown"
      >
        <div class="flex items-center justify-between px-4 h-16 border-b border-zinc-100 dark:border-zinc-800">
          <RouterLink to="/dashboard" class="flex items-center gap-2" @click="closeDrawer">
            <div class="w-7 h-7 rounded-lg bg-zinc-900 dark:bg-zinc-100 flex items-center justify-center">
              <svg class="w-4 h-4 text-white dark:text-zinc-900" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
              </svg>
            </div>
            <span class="font-bold tracking-tight text-zinc-900 dark:text-zinc-100">Noumena</span>
          </RouterLink>
          <button
            type="button"
            aria-label="Close navigation menu"
            @click="closeDrawer"
            class="flex items-center justify-center h-11 w-11 rounded-lg text-zinc-600 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800"
          >
            <X class="w-5 h-5" />
          </button>
        </div>

        <nav class="flex-1 overflow-y-auto px-3 py-4 space-y-1">
          <RouterLink
            v-for="item in navItems"
            :key="item.to"
            :to="item.to"
            @click="closeDrawer"
            :class="[
              isActive(item.to, item.exact)
                ? 'bg-zinc-100 dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100'
                : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-50 dark:hover:bg-zinc-800/50',
              'flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors',
            ]"
          >
            <component :is="item.icon" class="w-4 h-4" />
            {{ item.label }}
          </RouterLink>
        </nav>

        <div class="px-4 py-4 border-t border-zinc-100 dark:border-zinc-800 space-y-3">
          <WorkspaceSwitcher />
          <div class="space-y-1">
            <RouterLink
              to="/settings/account"
              role="menuitem"
              @click="closeDrawer"
              class="flex items-center gap-2 px-3 py-2 rounded-lg text-sm text-zinc-700 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-800"
            >
              <Settings class="w-4 h-4" /> Account
            </RouterLink>
            <RouterLink
              v-if="isAdmin"
              to="/settings/members"
              role="menuitem"
              @click="closeDrawer"
              class="flex items-center gap-2 px-3 py-2 rounded-lg text-sm text-zinc-700 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-800"
            >
              <Users class="w-4 h-4" /> Manage members
            </RouterLink>
            <button
              type="button"
              role="menuitem"
              @click="handleLogout"
              class="w-full text-left flex items-center gap-2 px-3 py-2 rounded-lg text-sm text-zinc-700 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-800"
            >
              <LogOut class="w-4 h-4" /> Log out
            </button>
          </div>
        </div>
      </aside>
    </transition>
  </div>
</template>

<style scoped>
.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity 0.2s ease;
}
.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}
.drawer-slide-enter-active,
.drawer-slide-leave-active {
  transition: transform 0.25s ease;
}
.drawer-slide-enter-from,
.drawer-slide-leave-to {
  transform: translateX(-100%);
}
.menu-enter-active,
.menu-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.menu-enter-from,
.menu-leave-to {
  opacity: 0;
  transform: translateY(-4px) scale(0.98);
}
</style>
