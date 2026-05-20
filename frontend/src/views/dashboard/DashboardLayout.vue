<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
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
} from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

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

async function handleLogout() {
  menuOpen.value = false
  await auth.logout()
  router.push('/login')
}

async function handleLogoutAll() {
  menuOpen.value = false
  await auth.logoutAll()
  router.push('/login')
}
</script>

<template>
  <div class="min-h-screen bg-zinc-50 flex">
    <!-- Sidebar -->
    <aside class="hidden lg:flex flex-col w-64 bg-white border-r border-zinc-200 sticky top-0 h-screen">
      <div class="px-6 py-5 border-b border-zinc-100">
        <RouterLink to="/dashboard" class="flex items-center gap-2 group">
          <div class="w-8 h-8 rounded-lg bg-zinc-900 flex items-center justify-center shadow-md group-hover:scale-105 transition-transform">
            <svg class="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
          </div>
          <span class="font-bold text-lg tracking-tight text-zinc-900">Noumena</span>
        </RouterLink>
      </div>

      <nav class="flex-1 px-3 py-4 space-y-1">
        <RouterLink
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          :class="[
            isActive(item.to, item.exact)
              ? 'bg-zinc-100 text-zinc-900'
              : 'text-zinc-600 hover:text-zinc-900 hover:bg-zinc-50',
            'flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors',
          ]"
        >
          <component :is="item.icon" class="w-4 h-4" />
          {{ item.label }}
        </RouterLink>
      </nav>

      <div class="px-6 py-4 border-t border-zinc-100 text-xs text-zinc-400">
        © {{ new Date().getFullYear() }} Noumena
      </div>
    </aside>

    <!-- Main column -->
    <div class="flex-1 flex flex-col min-w-0">
      <!-- Top bar -->
      <header class="bg-white/80 backdrop-blur-md border-b border-zinc-200 sticky top-0 z-30">
        <div class="px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between">
          <!-- Mobile brand -->
          <RouterLink to="/dashboard" class="lg:hidden flex items-center gap-2">
            <div class="w-7 h-7 rounded-lg bg-zinc-900 flex items-center justify-center">
              <svg class="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
              </svg>
            </div>
            <span class="font-bold tracking-tight text-zinc-900">Noumena</span>
          </RouterLink>
          <span class="hidden lg:block"></span>

          <div ref="menuRef" class="relative">
            <button
              @click.stop="toggleMenu"
              class="flex items-center gap-2 pl-2 pr-3 py-1.5 rounded-full border border-zinc-200 hover:border-zinc-300 hover:bg-zinc-50 transition-colors"
              :aria-expanded="menuOpen"
            >
              <span class="h-7 w-7 rounded-full bg-gradient-to-tr from-indigo-500 to-purple-500 text-white font-semibold text-xs flex items-center justify-center">
                {{ userInitial }}
              </span>
              <span class="hidden sm:inline text-sm font-medium text-zinc-700 max-w-[12rem] truncate">
                {{ auth.user?.email || 'Account' }}
              </span>
              <ChevronDown class="w-4 h-4 text-zinc-500" />
            </button>

            <transition name="menu">
              <div
                v-if="menuOpen"
                class="absolute right-0 mt-2 w-64 bg-white shadow-lg ring-1 ring-zinc-950/5 rounded-2xl py-2 origin-top-right"
              >
                <div class="px-4 py-3 border-b border-zinc-100">
                  <p class="text-xs font-medium text-zinc-500">Signed in as</p>
                  <p class="text-sm font-semibold text-zinc-900 truncate">{{ auth.user?.email || '—' }}</p>
                </div>
                <div class="py-1">
                  <RouterLink
                    to="/settings/account"
                    @click="menuOpen = false"
                    class="flex items-center gap-2 px-4 py-2 text-sm text-zinc-700 hover:bg-zinc-50"
                  >
                    <Settings class="w-4 h-4" /> Settings
                  </RouterLink>
                  <button
                    @click="handleLogout"
                    class="w-full text-left flex items-center gap-2 px-4 py-2 text-sm text-zinc-700 hover:bg-zinc-50"
                  >
                    <LogOut class="w-4 h-4" /> Log out
                  </button>
                  <button
                    @click="handleLogoutAll"
                    class="w-full text-left flex items-center gap-2 px-4 py-2 text-sm text-rose-600 hover:bg-rose-50"
                  >
                    <Users class="w-4 h-4" /> Log out of all devices
                  </button>
                </div>
              </div>
            </transition>
          </div>
        </div>
      </header>

      <main class="flex-1">
        <RouterView />
      </main>
    </div>
  </div>
</template>

<style scoped>
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
