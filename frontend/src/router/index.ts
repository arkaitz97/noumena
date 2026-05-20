import { createRouter, createWebHistory, type RouteLocationNormalized } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'landing',
      component: () => import('../views/public/LandingView.vue'),
      meta: { publicOnly: false },
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/auth/LoginView.vue'),
    },
    {
      path: '/signup',
      name: 'signup',
      component: () => import('../views/auth/SignupView.vue'),
    },
    {
      path: '/verify-email',
      name: 'verify',
      component: () => import('../views/auth/VerifyEmailView.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/forgot-password',
      name: 'forgot-password',
      component: () => import('../views/auth/ForgotPasswordView.vue'),
    },
    {
      path: '/reset-password',
      name: 'reset-password',
      component: () => import('../views/auth/ResetPasswordView.vue'),
    },
    {
      path: '/onboarding',
      name: 'onboarding',
      component: () => import('../views/auth/OnboardingView.vue'),
      meta: { requiresAuth: true, requiresVerified: true },
    },
    {
      path: '/oauth/callback',
      name: 'oauth-callback',
      component: () => import('../views/auth/OAuthCallbackView.vue'),
    },
    {
      path: '/dashboard',
      component: () => import('../views/dashboard/DashboardLayout.vue'),
      meta: { requiresAuth: true, requiresVerified: true, requiresOnboarded: true },
      children: [
        {
          path: '',
          name: 'dashboard',
          component: () => import('../views/dashboard/InsightsFeed.vue'),
        },
        {
          path: 'saved',
          name: 'saved-comments',
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
          component: () => import('../views/settings/ChannelsView.vue'),
        },
        {
          path: 'billing',
          name: 'billing',
          component: () => import('../views/settings/BillingView.vue'),
        },
        {
          path: 'account',
          name: 'account',
          component: () => import('../views/settings/AccountView.vue'),
        },
      ],
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

export default router
