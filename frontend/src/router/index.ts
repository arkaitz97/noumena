import { createRouter, createWebHistory, type RouteLocationNormalized } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'landing',
      meta: { title: 'Noumena', publicOnly: false },
      component: () => import('../views/public/LandingView.vue'),
    },
    {
      path: '/login',
      name: 'login',
      meta: { title: 'Log in' },
      component: () => import('../views/auth/LoginView.vue'),
    },
    {
      path: '/signup',
      name: 'signup',
      meta: { title: 'Sign up' },
      component: () => import('../views/auth/SignupView.vue'),
    },
    {
      path: '/verify-email',
      name: 'verify',
      meta: { title: 'Verify email', requiresAuth: true },
      component: () => import('../views/auth/VerifyEmailView.vue'),
    },
    {
      path: '/forgot-password',
      name: 'forgot-password',
      meta: { title: 'Reset password' },
      component: () => import('../views/auth/ForgotPasswordView.vue'),
    },
    {
      path: '/reset-password',
      name: 'reset-password',
      meta: { title: 'Reset password' },
      component: () => import('../views/auth/ResetPasswordView.vue'),
    },
    {
      path: '/onboarding',
      name: 'onboarding',
      meta: { title: 'Get started', requiresAuth: true, requiresVerified: true },
      component: () => import('../views/auth/OnboardingView.vue'),
    },
    {
      path: '/oauth/callback',
      name: 'oauth-callback',
      meta: { title: 'Connecting…' },
      component: () => import('../views/auth/OAuthCallbackView.vue'),
    },
    {
      path: '/invite',
      name: 'invite',
      meta: { title: 'Accept invite' },
      component: () => import('../views/auth/AcceptInviteView.vue'),
    },
    {
      path: '/dashboard',
      component: () => import('../views/dashboard/DashboardLayout.vue'),
      meta: { requiresAuth: true, requiresVerified: true, requiresOnboarded: true },
      children: [
        {
          path: '',
          name: 'dashboard',
      meta: { title: 'Insights Feed' },
          component: () => import('../views/dashboard/InsightsFeed.vue'),
        },
        {
          path: 'saved',
          name: 'saved-comments',
      meta: { title: 'Saved Comments' },
          component: () => import('../views/dashboard/SavedCommentsView.vue'),
        },
      ],
    },
    {
      path: '/settings',
      component: () => import('../views/dashboard/DashboardLayout.vue'),
      meta: { requiresAuth: true, requiresVerified: true, requiresOnboarded: true },
      children: [
        {
          path: 'channels',
          name: 'channels',
      meta: { title: 'Channels' },
          component: () => import('../views/settings/ChannelsView.vue'),
        },
        {
          path: 'members',
          name: 'members',
      meta: { title: 'Members' },
          component: () => import('../views/settings/MembersView.vue'),
        },
        {
          path: 'billing',
          name: 'billing',
      meta: { title: 'Billing' },
          component: () => import('../views/settings/BillingView.vue'),
        },
        {
          path: 'account',
          name: 'account',
      meta: { title: 'Account' },
          component: () => import('../views/settings/AccountView.vue'),
        },
      ],
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      redirect: '/',
    },
  ],
})

function metaFlag(to: RouteLocationNormalized, key: string): boolean {
  return to.matched.some((r) => Boolean((r.meta as Record<string, unknown> | undefined)?.[key]))
}

router.beforeEach((to) => {
  const auth = useAuthStore()

  const requiresAuth = metaFlag(to, 'requiresAuth')
  const requiresVerified = metaFlag(to, 'requiresVerified')
  const requiresOnboarded = metaFlag(to, 'requiresOnboarded')

  if (requiresAuth && !auth.isAuthenticated) {
    return { path: '/login', query: { next: to.fullPath } }
  }

  if (requiresVerified && auth.isAuthenticated && !auth.isEmailVerified) {
    if (to.path !== '/verify-email') return '/verify-email'
  }

  if (requiresOnboarded && auth.isAuthenticated && auth.isEmailVerified && !auth.hasChannels) {
    if (to.path !== '/onboarding') return '/onboarding'
  }

  // Authenticated users hitting login/signup → bounce to the right place.
  if (auth.isAuthenticated && (to.path === '/login' || to.path === '/signup')) {
    if (!auth.isEmailVerified) return '/verify-email'
    if (!auth.hasChannels) return '/onboarding'
    return '/dashboard'
  }

  return true
})

router.afterEach((to) => {
  if (typeof document === 'undefined') return
  const title = to.meta.title as string | undefined
  document.title = title ? `${title} · Noumena` : 'Noumena'

  // Strip the invite token from outbound Referer headers while the invite
  // route (or its login handoff) is on screen.
  const existing = document.head.querySelector('meta[name="referrer"]')
  if (to.name === 'invite') {
    if (!existing) {
      const meta = document.createElement('meta')
      meta.setAttribute('name', 'referrer')
      meta.setAttribute('content', 'no-referrer')
      meta.setAttribute('data-invite-referrer', '')
      document.head.appendChild(meta)
    }
  } else if (existing && existing.getAttribute('data-invite-referrer') !== null) {
    existing.remove()
  }
})

export default router
