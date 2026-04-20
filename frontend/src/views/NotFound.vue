<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()
</script>

<template>
  <div class="not-found">
    <div class="not-found__content">
      <div class="not-found__code">404</div>
      <h1 class="not-found__title">页面未找到</h1>
      <p class="not-found__desc">你访问的页面不存在或已被移除</p>
      <button class="not-found__btn" @click="router.push('/dashboard')">
        返回首页
      </button>
    </div>
  </div>
</template>

<style scoped>
.not-found {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 80vh;
  padding: 2rem;
}
.not-found__content {
  text-align: center;
}
.not-found__code {
  font-size: 6rem;
  font-weight: 900;
  background: linear-gradient(135deg, var(--sun, #FF8C00), var(--coral, #FF9F43));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  line-height: 1;
  margin-bottom: 0.5rem;
}
.not-found__title {
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--ink, #2D1B0E);
  margin-bottom: 0.5rem;
}
.not-found__desc {
  color: var(--ink-soft, #8D7060);
  margin-bottom: 2rem;
}
.not-found__btn {
  display: inline-block;
  padding: 0.625rem 2rem;
  border: none;
  border-radius: 9px;
  font-weight: 700;
  cursor: pointer;
  font-size: 0.875rem;
  background: linear-gradient(135deg, #FF8C00, #FF9F43);
  color: #fff;
  transition: all 0.18s ease;
}
.not-found__btn:hover {
  box-shadow: 0 4px 18px rgba(249, 115, 22, 0.36);
  transform: translateY(-1px);
}
</style>
