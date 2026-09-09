<script setup lang="ts">
import { computed, useAttrs } from 'vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  modelValue: string
  id?: string
  label?: string
  type?: string
  placeholder?: string
  autocomplete?: string
  required?: boolean
  disabled?: boolean
  error?: string | null
  hint?: string
}>()

const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()

const attrs = useAttrs()
const inputId = computed(() => props.id || `input-${Math.random().toString(36).slice(2, 8)}`)

const onInput = (e: Event) => {
  const target = e.target as HTMLInputElement
  emit('update:modelValue', target.value)
}
</script>

<template>
  <div>
    <label v-if="label" :for="inputId" class="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5">
      {{ label }}
    </label>
    <input
      v-bind="attrs"
      :id="inputId"
      :type="type || 'text'"
      :value="modelValue"
      :placeholder="placeholder"
      :autocomplete="autocomplete"
      :required="required"
      :disabled="disabled"
      :aria-invalid="error ? 'true' : undefined"
      :aria-describedby="error ? `${inputId}-error` : hint ? `${inputId}-hint` : undefined"
      @input="onInput"
      class="appearance-none block w-full px-4 py-3 border border-zinc-200 dark:border-white/10 rounded-xl bg-white dark:bg-zinc-800/60 shadow-sm placeholder-zinc-400 dark:placeholder-zinc-500 sm:text-sm transition-all duration-200 ease-snappy focus:outline-none focus:ring-2 disabled:opacity-60 disabled:cursor-not-allowed text-zinc-900 dark:text-zinc-100 hover:border-zinc-300 dark:hover:border-white/20"
      :class="
        error
          ? 'border-rose-400/70 focus:border-rose-500 focus:ring-rose-500/40'
          : 'focus:border-brand-500/70 focus:ring-brand-500/30'
      "
    />
    <p v-if="error" :id="`${inputId}-error`" role="alert" class="mt-1.5 text-xs text-rose-600 dark:text-rose-400 font-medium">{{ error }}</p>
    <p v-else-if="hint" :id="`${inputId}-hint`" class="mt-1.5 text-xs text-zinc-500 dark:text-zinc-400">{{ hint }}</p>
  </div>
</template>
