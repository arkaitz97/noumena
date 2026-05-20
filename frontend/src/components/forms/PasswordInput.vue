<script setup lang="ts">
import { computed, ref } from 'vue'
import { passwordRules, validatePassword } from '../../lib/validation'

const props = defineProps<{
  modelValue: string
  id?: string
  label?: string
  placeholder?: string
  autocomplete?: string
  required?: boolean
  disabled?: boolean
  showStrength?: boolean
  error?: string | null
}>()

const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()

const inputId = computed(() => props.id || `pw-${Math.random().toString(36).slice(2, 8)}`)
const visible = ref(false)

const validation = computed(() => validatePassword(props.modelValue))
const strengthClass = computed(() => {
  const s = validation.value.score
  if (s === 0) return 'bg-zinc-200'
  if (s === 1) return 'bg-rose-400'
  if (s === 2) return 'bg-amber-400'
  return 'bg-emerald-500'
})
const strengthLabel = computed(() => {
  const s = validation.value.score
  if (s === 0) return ''
  if (s === 1) return 'Weak'
  if (s === 2) return 'Okay'
  return 'Strong'
})

const onInput = (e: Event) => emit('update:modelValue', (e.target as HTMLInputElement).value)
</script>

<template>
  <div>
    <label v-if="label" :for="inputId" class="block text-sm font-medium text-zinc-700 mb-1.5">
      {{ label }}
    </label>
    <div class="relative">
      <input
        :id="inputId"
        :type="visible ? 'text' : 'password'"
        :value="modelValue"
        :placeholder="placeholder"
        :autocomplete="autocomplete || 'new-password'"
        :required="required"
        :disabled="disabled"
        @input="onInput"
        class="appearance-none block w-full px-4 py-3 pr-12 border border-zinc-200 rounded-xl shadow-sm placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900 sm:text-sm transition-colors bg-zinc-50/50 focus:bg-white text-zinc-900 disabled:opacity-60"
        :class="error ? 'border-rose-300 focus:ring-rose-500 focus:border-rose-500' : ''"
      />
      <button
        type="button"
        @click="visible = !visible"
        tabindex="-1"
        class="absolute inset-y-0 right-0 px-3 flex items-center text-xs font-medium text-zinc-500 hover:text-zinc-900 transition-colors"
        :aria-label="visible ? 'Hide password' : 'Show password'"
      >
        {{ visible ? 'Hide' : 'Show' }}
      </button>
    </div>

    <div v-if="showStrength && modelValue.length > 0" class="mt-3 space-y-2">
      <div class="flex items-center gap-2">
        <div class="flex-1 h-1.5 bg-zinc-100 rounded-full overflow-hidden">
          <div
            class="h-full transition-all duration-200"
            :class="strengthClass"
            :style="{ width: (validation.score / passwordRules.length) * 100 + '%' }"
          ></div>
        </div>
        <span class="text-xs font-medium text-zinc-500 min-w-[3rem] text-right">{{ strengthLabel }}</span>
      </div>
      <ul class="text-xs text-zinc-500 space-y-1 font-medium">
        <li
          v-for="rule in passwordRules"
          :key="rule.id"
          :class="validation.results[rule.id] ? 'text-emerald-600' : ''"
          class="flex items-center transition-colors"
        >
          <svg
            v-if="validation.results[rule.id]"
            class="w-3.5 h-3.5 mr-1.5"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
          </svg>
          <span v-else class="w-1.5 h-1.5 rounded-full bg-zinc-300 mr-2.5 ml-1 inline-block"></span>
          {{ rule.label }}
        </li>
      </ul>
    </div>

    <p v-if="error" class="mt-1.5 text-xs text-rose-600 font-medium">{{ error }}</p>
  </div>
</template>
