<script setup lang="ts">
import { RouterView } from 'vue-router'
import ToastHost from './components/ToastHost.vue'
import { useDarkMode } from './composables/useDarkMode'

useDarkMode()
</script>

<template>
  <RouterView />
  <ToastHost />
</template>
