<template>
  <div v-if="hasError" class="error-boundary">
    <div class="error-boundary__content">
      <div class="error-boundary__icon">⚠️</div>
      <h2 class="error-boundary__title">页面加载出错</h2>
      <p class="error-boundary__message">{{ errorMessage }}</p>
      <button class="error-boundary__btn" @click="reset">重试</button>
      <button class="error-boundary__btn error-boundary__btn--secondary" @click="goHome">返回首页</button>
    </div>
  </div>
  <slot v-else />
</template>

<script lang="ts">
import { defineComponent } from 'vue'

export default defineComponent({
  name: 'ErrorBoundary',
  data() {
    return {
      hasError: false,
      errorMessage: '',
    }
  },
  methods: {
    reset() {
      this.hasError = false
      this.errorMessage = ''
    },
    goHome() {
      window.location.href = '/dashboard'
    },
  },
  errorCaptured(err: Error, _instance: any, info: string) {
    this.hasError = true
    this.errorMessage = err.message || '未知错误'
    console.error('[ErrorBoundary]', err, info)
    return false
  },
})
</script>

<style scoped>
.error-boundary {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 400px;
  padding: 2rem;
}
.error-boundary__content { text-align: center; max-width: 400px; }
.error-boundary__icon { font-size: 3rem; margin-bottom: 1rem; }
.error-boundary__title { font-size: 1.25rem; font-weight: 700; color: var(--ink, #2D1B0E); margin-bottom: 0.5rem; }
.error-boundary__message { color: var(--ink-soft, #8D7060); margin-bottom: 1.5rem; word-break: break-word; }
.error-boundary__btn {
  display: inline-block; padding: 0.5rem 1.5rem; margin: 0 0.25rem;
  border: none; border-radius: 9px; font-weight: 700; cursor: pointer; font-size: 0.875rem;
  background: linear-gradient(135deg, #FF8C00, #FF9F43); color: #fff; transition: all 0.18s ease;
}
.error-boundary__btn:hover { box-shadow: 0 4px 18px rgba(249, 115, 22, 0.36); }
.error-boundary__btn--secondary { background: #f5f0eb; color: var(--ink-mid, #5D4037); }
.error-boundary__btn--secondary:hover { background: #ede5dd; box-shadow: none; }
</style>
