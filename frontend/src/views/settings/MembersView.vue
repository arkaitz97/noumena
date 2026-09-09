<script setup lang="ts">
// Members (/settings/members) — workspace member roster + invite / role /
// remove. Admin-only: the backend enforces role (403), the UI hides the
// controls for non-admins.
//
// Seat usage (§4.3): the roster header shows "X of Y seats" from the
// member-list envelope (`member_count` / `seats_limit`). When a legacy
// workspace sits over its plan cap the pill flips to a warning state but
// the roster still renders.

import { computed, onMounted, ref } from 'vue'
import { UserPlus, Users, ShieldCheck } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'
import { useWorkspaceRole } from '../../composables/useWorkspaceRole'
import { toast } from '../../lib/toast'
import apiClient from '../../services/api'
import {
  changeMemberRole,
  inviteMember,
  listMembers,
  removeMember,
  type MemberRole,
  type WorkspaceMember,
  type WorkspaceSummary,
} from '../../services/workspaces'

const auth = useAuthStore()
const { isAdmin } = useWorkspaceRole()

const workspaces = ref<WorkspaceSummary[]>([])
const activeWorkspace = computed(() => workspaces.value.find((w) => w.is_active) ?? null)
const members = ref<WorkspaceMember[]>([])
const memberCount = ref<number | null>(null)
const seatsLimit = ref<number | null>(null)
const loading = ref(false)

const usedSeats = computed(() => memberCount.value ?? members.value.length)
const seatsOverBy = computed(() =>
  seatsLimit.value === null ? 0 : Math.max(0, usedSeats.value - seatsLimit.value),
)
const seatsOver = computed(() => seatsLimit.value !== null && seatsOverBy.value > 0)

const seatsBadgeClass = computed(() =>
  seatsOver.value
    ? 'bg-amber-50 text-amber-700 ring-amber-600/20 dark:bg-amber-400/10 dark:text-amber-300 dark:ring-amber-400/20' // unslop-ignore: semantic warning
    : 'bg-emerald-50 text-emerald-700 ring-emerald-600/20 dark:bg-emerald-400/10 dark:text-emerald-300 dark:ring-emerald-400/20',
)

const inviteEmail = ref('')
const inviteRole = ref<MemberRole>('viewer')
const inviteError = ref<string | null>(null)
const inviting = ref(false)
const changingId = ref<string | null>(null)
const removingId = ref<string | null>(null)

const ROLES: MemberRole[] = ['admin', 'analyst', 'viewer']

async function loadMembers(): Promise<void> {
  const ws = activeWorkspace.value
  if (!ws) return
  loading.value = true
  try {
    const res = await listMembers(ws.id)
    members.value = res.members
    memberCount.value = res.member_count
    seatsLimit.value = res.seats_limit ?? ws.seats_limit ?? null
  } catch {
    toast.error('Could not load members.')
  } finally {
    loading.value = false
  }
}

async function load(): Promise<void> {
  try {
    const { data } = await apiClient.get<WorkspaceSummary[]>('/workspaces/mine')
    workspaces.value = data ?? []
  } catch {
    workspaces.value = []
  }
  await loadMembers()
}

onMounted(() => {
  void load()
})

async function onInvite(): Promise<void> {
  const ws = activeWorkspace.value
  if (!ws) return
  const email = inviteEmail.value.trim().toLowerCase()
  inviteError.value = null
  if (!email) {
    inviteError.value = 'Email is required.'
    return
  }
  inviting.value = true
  try {
    await inviteMember(ws.id, email, inviteRole.value)
    inviteEmail.value = ''
    inviteRole.value = 'viewer'
    toast.success(`Invite sent to ${email}.`)
  } catch (err: any) {
    inviteError.value = err?.response?.data?.error || 'Could not create the invite.'
  } finally {
    inviting.value = false
  }
}

async function onRoleChange(m: WorkspaceMember, e: Event): Promise<void> {
  const ws = activeWorkspace.value
  if (!ws) return
  const role = (e.target as HTMLSelectElement).value as MemberRole
  changingId.value = m.user_id
  try {
    await changeMemberRole(ws.id, m.user_id, role)
    m.role = role
    toast.success('Role updated.')
  } catch (err: any) {
    // 409 from the backend carries the real reason (e.g. seat limit
    // reached) — surface it instead of a generic network error.
    toast.error(err?.response?.data?.error || 'Could not update the role.')
  } finally {
    changingId.value = null
  }
}

async function onRemove(m: WorkspaceMember): Promise<void> {
  const ws = activeWorkspace.value
  if (!ws) return
  if (!window.confirm(`Remove ${m.email} from this workspace?`)) return
  removingId.value = m.user_id
  try {
    await removeMember(ws.id, m.user_id)
    members.value = members.value.filter((x) => x.user_id !== m.user_id)
    toast.success('Member removed.')
  } catch {
    toast.error('Could not remove the member.')
  } finally {
    removingId.value = null
  }
}
</script>

<template>
  <div class="max-w-5xl mx-auto py-8 px-4 sm:px-6 lg:px-8 space-y-8">
    <div>
      <h1 class="text-2xl font-semibold leading-7 text-zinc-900 dark:text-white tracking-tight">Members</h1>
      <p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
        Manage who can access
        <strong class="font-medium text-zinc-700 dark:text-zinc-300">{{ activeWorkspace?.name ?? 'this workspace' }}</strong>.
      </p>
    </div>

    <div v-if="!isAdmin" class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl p-8 text-center">
      <Users class="mx-auto h-10 w-10 text-zinc-300 mb-3" />
      <h2 class="text-sm font-semibold text-zinc-900 dark:text-white mb-1">Admins only</h2>
      <p class="text-sm text-zinc-500 dark:text-zinc-400 max-w-sm mx-auto">
        You need the admin role in this workspace to manage members.
      </p>
    </div>

    <template v-else>
      <!-- Invite -->
      <section class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl p-6 sm:p-8">
        <div class="md:grid md:grid-cols-3 md:gap-8">
          <div class="md:col-span-1">
            <h2 class="text-base font-semibold text-zinc-900 dark:text-white">Invite a member</h2>
            <p class="mt-1 text-sm text-zinc-500 pr-4">They'll receive an email with a join link.</p>
          </div>
          <form class="mt-5 md:mt-0 md:col-span-2 space-y-3" @submit.prevent="onInvite">
            <div class="flex flex-col sm:flex-row gap-3">
              <input
                v-model="inviteEmail"
                type="email"
                autocomplete="email"
                placeholder="teammate@example.com"
                class="flex-1 appearance-none px-4 py-2.5 border border-zinc-200/70 dark:border-white/5 rounded-xl shadow-sm text-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60 focus-visible:border-brand-500/70 bg-zinc-50/50 dark:bg-zinc-800 dark:text-white"
              />
              <select
                v-model="inviteRole"
                class="px-3 py-2.5 border border-zinc-200/70 dark:border-white/5 rounded-xl text-sm bg-zinc-50/50 dark:bg-zinc-800 dark:text-white focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
                aria-label="Invite role"
              >
                <option value="viewer">Viewer</option>
                <option value="analyst">Analyst</option>
                <option value="admin">Admin</option>
              </select>
              <button
                type="submit"
                :disabled="inviting"
                class="inline-flex items-center justify-center gap-1.5 px-4 py-2.5 rounded-xl text-sm font-medium text-white bg-zinc-900 hover:bg-zinc-700 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200 shadow-card dark:shadow-none transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <UserPlus class="w-4 h-4" />
                {{ inviting ? 'Sending…' : 'Invite' }}
              </button>
            </div>
            <p v-if="inviteError" role="alert" class="text-xs text-rose-600 dark:text-rose-400 font-medium">{{ inviteError }}</p>
          </form>
        </div>
      </section>

      <!-- Roster -->
      <section class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl overflow-hidden">
        <div class="px-6 py-4 border-b border-zinc-200/70 dark:border-white/5 flex items-center gap-2">
          <ShieldCheck class="w-4 h-4 text-zinc-400" />
          <h2 class="text-sm font-semibold text-zinc-900 dark:text-white">Roster</h2>
          <span
            v-if="seatsLimit !== null"
            :class="[
              'ml-auto inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[10px] font-semibold uppercase tracking-wider ring-1 ring-inset',
              seatsBadgeClass,
            ]"
            :title="`${usedSeats} of ${seatsLimit} seats used`"
          >
            {{ usedSeats }} of {{ seatsLimit }} seats
            <span v-if="seatsOver">+{{ seatsOverBy }} over plan limit</span>
          </span>
          <span v-else class="ml-auto text-xs text-zinc-500 dark:text-zinc-400">{{ members.length }} member{{ members.length === 1 ? '' : 's' }}</span>
        </div>
        <div v-if="loading" class="p-6 text-sm text-zinc-500 dark:text-zinc-400">Loading…</div>
        <ul v-else-if="members.length > 0" class="divide-y divide-zinc-200/70 dark:divide-white/5">
          <li v-for="m in members" :key="m.user_id" class="px-6 py-4 flex items-center gap-3">
            <div class="flex-1 min-w-0">
              <p class="text-sm font-medium text-zinc-900 dark:text-white truncate">
                {{ m.email }}
                <span v-if="m.user_id === auth.user?.id" class="text-[10px] uppercase tracking-wider text-zinc-400 ml-1">you</span>
              </p>
            </div>
            <select
              :value="m.role"
              :disabled="changingId === m.user_id || m.user_id === auth.user?.id"
              @change="onRoleChange(m, $event)"
              class="px-2 py-1.5 text-xs border border-zinc-200/70 dark:border-white/5 rounded-lg bg-zinc-50/50 dark:bg-zinc-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60 disabled:opacity-40"
              :aria-label="`Role for ${m.email}`"
            >
              <option v-for="r in ROLES" :key="r" :value="r" class="capitalize">{{ r }}</option>
            </select>
            <button
              type="button"
              :disabled="removingId === m.user_id || m.user_id === auth.user?.id"
              @click="onRemove(m)"
              class="px-3 py-1.5 rounded-lg text-xs font-medium text-rose-600 dark:text-rose-400 bg-rose-50 dark:bg-rose-500/10 hover:bg-rose-100 dark:hover:bg-rose-500/20 disabled:opacity-40 focus:outline-none focus-visible:ring-2 focus-visible:ring-rose-500"
            >
              Remove
            </button>
          </li>
        </ul>
        <div v-else class="p-6 text-sm text-zinc-500 dark:text-zinc-400">No members yet.</div>
      </section>
    </template>
  </div>
</template>