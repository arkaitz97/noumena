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
import { useAccountStore } from '../../stores/account'
import { useDarkMode } from '../../composables/useDarkMode'
import { useWorkspaceRole } from '../../composables/useWorkspaceRole'
import { useRateLimitStore } from '../../stores/rateLimit'
import { toast } from '../../lib/toast'
import WorkspaceSwitcher from '../../components/WorkspaceSwitcher.vue'
import RateLimitBanner from '../../components/RateLimitBanner.vue'
import TokenExpiryBanner from '../../components/TokenExpiryBanner.vue'
import AppLogo from '../../components/AppLogo.vue'
import { usePolling } from '../../lib/usePolling'
import { getTokenExpiry } from '../../services/tokenExpiry'
import type { TokenExpiryChannel } from '../../services/tokenExpiry'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const account = useAccountStore()
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
  if (event.target && menuRef.value.contains(event.target as Node)) return
  menuOpen.value = false
}

document.addEventListener('click', closeMenuOnOutside)
onBeforeUnmount(() => document.removeEventListener('click', closeMenuOnOutside))

const userDisplayName = computed(() => {
  return account.creatorProfile?.display_name?.trim() || auth.user?.email || 'Account'
})

const userInitial = computed(() => {
  const name = account.creatorProfile?.display_name?.trim() || auth.user?.email || '?'
  return name.slice(0, 1).toUpperCase()
})

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
  if (!account.creatorProfile) {
    void account.fetchCreatorProfile()
  }
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
          <div class="px-5 py-5 border-b border-zinc-100 dark:border-white/5">
        <RouterLink to="/dashboard" class="flex items-center group focus:outline-none">
          <AppLogo size="md" show-text />
        </RouterLink>
      </div>

      <div class="px-3 py-4 border-b border-zinc-100 dark:border-white/5">
        <WorkspaceSwitcher />
      </div>

      <nav class="flex-1 px-3 py-5 space-y-1">
        <RouterLink
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          :class="[
            isActive(item.to, item.exact)
              ? 'bg-white dark:bg-zinc-800 text-zinc-900 dark:text-white shadow-card dark:shadow-none ring-1 ring-zinc-950/5 dark:ring-white/5'
              : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-100/80 dark:hover:bg-white/5',
            'group flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-all duration-200 ease-snappy focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand-500/50',
          ]"
        >
          <component
            :is="item.icon"
            class="w-4 h-4 shrink-0"
            :class="isActive(item.to, item.exact) ? 'text-brand-600 dark:text-brand-400' : 'text-zinc-400 dark:text-zinc-500 group-hover:text-zinc-600'"
          />
          {{ item.label }}
        </RouterLink>
      </nav>

      <div class="px-3 py-4 border-t border-zinc-100 dark:border-zinc-800 space-y-3">
        <div ref="menuRef" class="relative" @keydown="onMenuKeydown">
          <div class="flex items-center gap-2">
            <button
              ref="menuTriggerRef"
              @click.stop="toggleMenu"
              class="flex-1 min-w-0 flex items-center gap-2 pl-1.5 pr-2.5 py-1.5 min-h-10 rounded-full border border-zinc-200 dark:border-white/10 hover:border-zinc-300 dark:hover:border-white/20 hover:bg-zinc-50 dark:hover:bg-zinc-800/60 transition-all duration-200 ease-snappy active:scale-[0.98] shadow-card dark:shadow-none"
              :aria-expanded="menuOpen"
              aria-haspopup="menu"
            >
              <span class="h-7 w-7 shrink-0 rounded-full bg-gradient-to-br from-brand-400 to-brand-600 text-white font-bold text-xs flex items-center justify-center shadow-card ring-1 ring-white/20">
                {{ userInitial }}
              </span>
              <span class="flex-1 min-w-0 text-left text-sm font-medium text-zinc-700 dark:text-zinc-300 truncate">
                {{ userDisplayName }}
              </span>
              <ChevronDown class="w-4 h-4 shrink-0 text-zinc-500 dark:text-zinc-400" :class="menuOpen && 'rotate-180'" />
            </button>

            <button
              @click="toggleDarkMode"
              class="flex items-center justify-center h-10 w-10 shrink-0 rounded-lg text-zinc-500 dark:text-zinc-400 hover:bg-zinc-100/80 dark:hover:bg-white/5 hover:text-zinc-900 dark:hover:text-white transition-all duration-200 active:scale-95"
              :aria-label="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
            >
              <Sun v-if="isDark" class="w-4 h-4" />
              <Moon v-else class="w-4 h-4" />
            </button>
          </div>

          <transition name="menu">
            <div
              v-if="menuOpen"
              role="menu"
              aria-label="User menu"
              class="absolute left-0 bottom-full mb-2 w-60 bg-white dark:bg-zinc-900 shadow-pop ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl py-2 origin-bottom-left z-50"
            >
              <div class="px-4 py-3 border-b border-zinc-100 dark:border-zinc-800">
                <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400">Signed in as</p>
                <p class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 truncate">{{ userDisplayName }}</p>
                <p v-if="account.creatorProfile?.display_name?.trim() && auth.user?.email" class="text-xs text-zinc-500 dark:text-zinc-400 truncate mt-0.5">
                  {{ auth.user.email }}
                </p>
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

        <p class="px-3 text-[11px] text-zinc-400 dark:text-zinc-500">© {{ new Date().getFullYear() }} Noumena</p>
      </div>
    </aside>

    <!-- Main column -->
    <div class="flex-1 flex flex-col min-w-0">
      <!-- Mobile top bar -->
      <div class="lg:hidden sticky top-0 z-30 bg-white/70 dark:bg-zinc-950/70 backdrop-blur-xl border-b border-zinc-200/60 dark:border-white/5 supports-[backdrop-filter]:bg-white/60 dark:supports-[backdrop-filter]:bg-zinc-900/60">
        <div class="px-4 sm:px-6 h-16 flex items-center justify-between gap-4">
          <div class="flex items-center gap-2 min-w-0">
            <button
              ref="hamburgerRef"
              type="button"
              aria-label="Open navigation menu"
              :aria-expanded="drawerOpen"
              aria-controls="mobile-nav-drawer"
              @click="openDrawer"
              class="flex items-center justify-center h-10 w-10 -ml-1.5 rounded-lg text-zinc-500 dark:text-zinc-400 hover:bg-zinc-100/80 dark:hover:bg-white/5 transition-colors duration-200"
            >
              <Menu class="w-5 h-5" />
            </button>
            <RouterLink to="/dashboard" class="flex items-center min-w-0">
              <AppLogo size="sm" show-text />
            </RouterLink>
          </div>

          <button
            @click="toggleDarkMode"
            class="flex items-center justify-center h-10 w-10 shrink-0 rounded-lg text-zinc-500 dark:text-zinc-400 hover:bg-zinc-100/80 dark:hover:bg-white/5 hover:text-zinc-900 dark:hover:text-white transition-all duration-200 active:scale-95"
            :aria-label="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
          >
            <Sun v-if="isDark" class="w-4 h-4" />
            <Moon v-else class="w-4 h-4" />
          </button>
        </div>
      </div>

      <!-- Notifications -->
      <div v-if="rateLimit.visible || (!tokenExpiryDismissed && expiringChannels.length > 0)" class="px-4 sm:px-6 lg:px-8 py-3 space-y-2">
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
          <RouterLink to="/dashboard" class="flex items-center" @click="closeDrawer">
            <AppLogo size="sm" show-text />
          </RouterLink>
          <button
            type="button"
            aria-label="Close navigation menu"
            @click="closeDrawer"
            class="flex items-center justify-center h-10 w-10 rounded-lg text-zinc-500 dark:text-zinc-400 hover:bg-zinc-100/80 dark:hover:bg-white/5"
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
                ? 'bg-white dark:bg-zinc-800 text-zinc-900 dark:text-white shadow-card dark:shadow-none ring-1 ring-zinc-950/5 dark:ring-white/5'
                : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-100/80 dark:hover:bg-white/5',
              'flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-all duration-200 ease-snappy focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand-500/50',
            ]"
          >
            <component
              :is="item.icon"
              class="w-4 h-4 shrink-0"
              :class="isActive(item.to, item.exact) ? 'text-brand-600 dark:text-brand-400' : 'text-zinc-400 dark:text-zinc-500'"
            />
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
  transition: opacity 0.25s cubic-bezier(0.16, 1, 0.3, 1);
}
.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}
.drawer-slide-enter-active,
.drawer-slide-leave-active {
  transition: transform 0.35s cubic-bezier(0.16, 1, 0.3, 1);
}
.drawer-slide-enter-from,
.drawer-slide-leave-to {
  transform: translateX(-100%);
}
.menu-enter-active,
.menu-leave-active {
  transition: opacity 0.18s cubic-bezier(0.16, 1, 0.3, 1), transform 0.18s cubic-bezier(0.16, 1, 0.3, 1);
}
.menu-enter-from,
.menu-leave-to {
  opacity: 0;
  transform: translateY(-6px) scale(0.97);
}
</style>
