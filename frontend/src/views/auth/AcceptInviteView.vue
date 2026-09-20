<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { useAuthStore } from '../../stores/auth'
import { fetchCurrentWorkspaceRole } from '../../composables/useWorkspaceRole'
import { acceptInvite, previewInvite, type InvitePreview } from '../../services/workspaces'
import {
  clearPendingInviteToken,
  getPendingInviteToken,
  setPendingInviteToken,
} from '../../lib/inviteToken'
import AppLogo from '../../components/AppLogo.vue'
import PrimaryButton from '../../components/forms/PrimaryButton.vue'
import SecondaryButton from '../../components/forms/SecondaryButton.vue'

type Phase = 'loading' | 'accepting' | 'success' | 'error'
type ErrorKind = 'missing' | 'expired' | 'wrong_account' | 'seat_limit' | 'generic'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const phase = ref<Phase>('loading')
const errorKind = ref<ErrorKind>('generic')
const preview = ref<InvitePreview | null>(null)
const acceptedWorkspace = ref<string>('')
const currentToken = ref<string>('')
const retrying = ref(false)

function fail(kind: ErrorKind): void {
  errorKind.value = kind
  phase.value = 'error'
  // Expired/consumed invites and full workspaces are terminal; a wrong
  // account keeps the token so the visitor can switch accounts, and
  // transient failures stay retryable.
  if (kind === 'expired' || kind === 'seat_limit') clearPendingInviteToken()
}

async function accept(token: string): Promise<void> {
  phase.value = 'accepting'
  try {
    const res = await acceptInvite(token)
    acceptedWorkspace.value = res.workspace_name
    clearPendingInviteToken()
    phase.value = 'success'

    await auth.fetchUser()
    await auth.refreshChannelsCount()
    await fetchCurrentWorkspaceRole()

    await router.replace(auth.hasChannels ? '/dashboard' : '/onboarding')
  } catch (err) {
    const status = (err as { response?: { status?: number } }).response?.status
    const code = (err as { response?: { data?: { code?: string } } }).response?.data?.code

    if (code === 'email_not_verified') {
      // Keep the pending token; the flow resumes at /invite after verification.
      await router.replace('/verify-email')
      return
    }
    if (status === 403) return fail('wrong_account')
    if (status === 409) return fail('seat_limit')
    if (status === 410) return fail('expired')
    fail('generic')
  }
}

async function run(token: string): Promise<void> {
  currentToken.value = token
  phase.value = 'loading'
  try {
    preview.value = await previewInvite(token)
  } catch (err) {
    const status = (err as { response?: { status?: number } }).response?.status
    if (status === 410) return fail('expired')
    return fail('generic')
  }

  if (!auth.isAuthenticated) {
    // Hand off to login; `next` brings the visitor back to complete acceptance.
    await router.replace({
      name: 'login',
      query: { next: `/invite?token=${encodeURIComponent(token)}` },
    })
    return
  }

  await accept(token)
}

async function switchAccount(): Promise<void> {
  const token = getPendingInviteToken() || currentToken.value
  try {
    await auth.logout()
  } catch {
    // Even if server logout fails, continue to the login handoff.
  }
  await router.push({
    name: 'login',
    query: token ? { next: `/invite?token=${encodeURIComponent(token)}` } : {},
  })
}

async function retry(): Promise<void> {
  retrying.value = true
  try {
    await run(getPendingInviteToken() || currentToken.value)
  } finally {
    retrying.value = false
  }
}

onMounted(async () => {
  let token = (route.query.token as string | undefined) || ''
  if (token) {
    setPendingInviteToken(token)
    // Strip the token from the address bar before any authenticated request.
    await router.replace({ path: '/invite' })
  } else {
    token = getPendingInviteToken() || ''
  }

  if (!token) {
    fail('missing')
    return
  }
  await run(token)
})
</script>

<template>
  <div class="min-h-screen bg-zinc-50 flex flex-col justify-center py-12 sm:px-6 lg:px-8 relative overflow-hidden">
    <div class="absolute top-[-10%] left-[10%] w-[30rem] h-[30rem] bg-brand-200 rounded-full mix-blend-multiply filter blur-[100px] opacity-40"></div>
    <div class="absolute bottom-[-10%] right-[10%] w-[30rem] h-[30rem] bg-blue-100 rounded-full mix-blend-multiply filter blur-[100px] opacity-40"></div>

    <div class="sm:mx-auto sm:w-full sm:max-w-md relative z-10">
      <div class="flex items-center justify-center mb-10">
        <AppLogo size="lg" show-text />
      </div>

      <div class="bg-white py-10 px-6 shadow-card rounded-xl border border-zinc-100/80 mx-4 sm:mx-0 text-center">
        <!-- Validating -->
        <template v-if="phase === 'loading'">
          <h2 class="text-2xl font-semibold tracking-tight text-zinc-900 mb-3">Checking your invite…</h2>
          <p class="text-zinc-500">One moment while we verify the link.</p>
        </template>

        <!-- Accepting -->
        <template v-else-if="phase === 'accepting'">
          <h2 class="text-2xl font-semibold tracking-tight text-zinc-900 mb-3">
            Joining <span v-if="preview">{{ preview.workspace_name }}</span><span v-else>your workspace</span>…
          </h2>
          <p class="text-zinc-500">Adding you to the workspace.</p>
        </template>

        <!-- Success (transient, then redirected) -->
        <template v-else-if="phase === 'success'">
          <h2 class="text-2xl font-semibold tracking-tight text-zinc-900 mb-3">
            You're in<template v-if="acceptedWorkspace">, {{ acceptedWorkspace }}</template>!
          </h2>
          <p class="text-zinc-500">Taking you to your dashboard…</p>
        </template>

        <!-- Missing token -->
        <template v-else-if="errorKind === 'missing'">
          <h2 class="text-2xl font-semibold tracking-tight text-zinc-900 mb-3">Invite link missing</h2>
          <p class="text-zinc-500 mb-8">
            Open the invitation link from your email to join the workspace.
          </p>
          <RouterLink to="/">
            <PrimaryButton full-width>Back to Noumena</PrimaryButton>
          </RouterLink>
        </template>

        <!-- Expired / consumed -->
        <template v-else-if="errorKind === 'expired'">
          <h2 class="text-2xl font-semibold tracking-tight text-zinc-900 mb-3">Invite expired or already used</h2>
          <p class="text-zinc-500 mb-8">
            This invitation is no longer valid. Ask a workspace Admin to send you a new invite.
          </p>
          <RouterLink to="/">
            <SecondaryButton full-width>Back to Noumena</SecondaryButton>
          </RouterLink>
        </template>

        <!-- Wrong account -->
        <template v-else-if="errorKind === 'wrong_account'">
          <h2 class="text-2xl font-semibold tracking-tight text-zinc-900 mb-3">This invite is for a different account</h2>
          <p class="text-zinc-500 mb-2">
            <template v-if="preview">
              This invite is for <span class="font-medium text-zinc-900">{{ preview.email }}</span>.
            </template>
            <template v-else>This invite belongs to another email address.</template>
          </p>
          <p class="text-zinc-500 mb-8">Sign out and sign back in with the invited email to accept it.</p>
          <PrimaryButton full-width @click="switchAccount">Switch account</PrimaryButton>
        </template>

        <!-- Seat limit -->
        <template v-else-if="errorKind === 'seat_limit'">
          <h2 class="text-2xl font-semibold tracking-tight text-zinc-900 mb-3">No free seats</h2>
          <p class="text-zinc-500 mb-8">
            This workspace has reached its plan's seat limit. An Admin needs to upgrade the plan
            before you can join.
          </p>
          <RouterLink to="/">
            <SecondaryButton full-width>Back to Noumena</SecondaryButton>
          </RouterLink>
        </template>

        <!-- Generic / transient -->
        <template v-else>
          <h2 class="text-2xl font-semibold tracking-tight text-zinc-900 mb-3">Couldn't accept the invite</h2>
          <p class="text-zinc-500 mb-8">Something went wrong. Check your connection and try again.</p>
          <PrimaryButton full-width :loading="retrying" @click="retry">Try again</PrimaryButton>
        </template>
      </div>
    </div>
  </div>
</template>
