import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'landing',
      component: () => import('../views/public/LandingView.vue')
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/auth/LoginView.vue')
    },
    {
      path: '/signup',
      name: 'signup',
      component: () => import('../views/auth/SignupView.vue')
    },
    {
      path: '/verify-email',
      name: 'verify',
      component: () => import('../views/auth/VerifyEmailView.vue')
    },
    {
      path: '/forgot-password',
      name: 'forgot-password',
      component: () => import('../views/auth/ForgotPasswordView.vue')
    },
    {
      path: '/reset-password',
      name: 'reset-password',
      component: () => import('../views/auth/ResetPasswordView.vue')
    },
    {
      path: '/onboarding',
      name: 'onboarding',
      component: () => import('../views/auth/OnboardingView.vue')
    },
    {
      path: '/dashboard',
      component: () => import('../views/dashboard/DashboardLayout.vue'),
      children: [
        {
          path: '',
          name: 'dashboard',
          component: () => import('../views/dashboard/InsightsFeed.vue')
        },
        {
          path: 'saved',
          name: 'saved-comments',
          component: () => import('../views/dashboard/SavedCommentsView.vue')
        }
      ]
    },
    {
      path: '/settings',
      component: () => import('../views/settings/SettingsLayout.vue'),
      children: [
        {
          path: 'channels',
          name: 'channels',
          component: () => import('../views/settings/ChannelsView.vue')
        },
        {
          path: 'billing',
          name: 'billing',
          component: () => import('../views/settings/BillingView.vue')
        },
        {
          path: 'account',
          name: 'account',
          component: () => import('../views/settings/AccountView.vue')
        }
      ]
    }
  ]
})

router.beforeEach((to) => {
  // TODO: Replace with real auth store logic
  const isAuthenticated = true // Replace with check for token
  const isVerified = true      // Replace with check for email_verified_at
  const hasChannels = localStorage.getItem('hasChannels') === 'true' // Replace with check for user channels

  const publicRoutes = ['/', '/login', '/signup', '/verify-email', '/forgot-password', '/reset-password']
  const isPublic = publicRoutes.includes(to.path)

  if (!isPublic) {
    if (!isAuthenticated) {
      return '/login'
    }
    if (!isVerified && to.path !== '/verify-email') {
      return '/verify-email'
    }
    if (isVerified && !hasChannels && to.path !== '/onboarding') {
      return '/onboarding'
    }
  }

  return true
})

export default router
