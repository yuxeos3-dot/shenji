<script>
export default { name: 'CacheManagement' }
</script>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const caches = ref([])
const loading = ref(false)
const clearing = ref({})

async function loadCaches() {
  loading.value = true
  try {
    const res = await api.listCaches()
    caches.value = res.data || []
  } catch (e) {
    console.error(e)
  }
  loading.value = false
}

async function clearCache(entry) {
  try {
    await ElMessageBox.confirm(
      `确定清空「${entry.label}」？\n\n${entry.description}`,
      '确认清空缓存',
      { type: 'warning', confirmButtonText: '清空', cancelButtonText: '取消' },
    )
  } catch { return }
  clearing.value[entry.key] = true
  try {
    await api.clearCache(entry.key)
    ElMessage.success(`${entry.label} 已清空`)
    await loadCaches()
  } catch (e) {
    console.error(e)
  }
  clearing.value[entry.key] = false
}

onMounted(loadCaches)
</script>

<template>
  <div class="cache-page">
    <div class="panel toolbar">
      <span class="toolbar-label">内存缓存概览</span>
      <span class="toolbar-hint">所有 in-memory 缓存均在进程内，进程重启即全部失效</span>
      <button class="btn btn-sm" @click="loadCaches" :disabled="loading" style="margin-left:auto">
        {{ loading ? '刷新中...' : '↻ 刷新' }}
      </button>
    </div>

    <div class="cache-grid">
      <div v-for="c in caches" :key="c.key" class="panel cache-card">
        <div class="cache-head">
          <div class="cache-name">{{ c.label }}</div>
          <code class="cache-key">{{ c.key }}</code>
        </div>
        <div class="cache-size-row">
          <div class="cache-size">
            <div class="size-num">{{ c.size }}</div>
            <div class="size-label">条目</div>
          </div>
          <div v-if="c.extra" class="cache-extra">{{ c.extra }}</div>
        </div>
        <div class="cache-desc">{{ c.description }}</div>
        <div class="cache-actions">
          <button
            class="btn btn-danger btn-sm"
            :disabled="!c.can_clear || clearing[c.key]"
            @click="clearCache(c)"
          >
            {{ clearing[c.key] ? '清空中...' : '清空' }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="!loading && caches.length === 0" class="panel empty-hint">
      <div class="empty-icon">💾</div>
      <div>暂无缓存数据</div>
    </div>
  </div>
</template>

<style scoped>
.cache-page { display: flex; flex-direction: column; gap: 16px; }
.panel { background: var(--surface); border-radius: var(--r); box-shadow: var(--s1); border: 1.5px solid var(--border-light); padding: 16px 22px; }

.toolbar { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; }
.toolbar-label { font-size: 14px; font-weight: 700; color: var(--ink); }
.toolbar-hint { font-size: 12px; color: var(--ink-hint); }

.cache-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.cache-card { display: flex; flex-direction: column; gap: 10px; }
.cache-head { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; }
.cache-name { font-size: 15px; font-weight: 800; color: var(--ink); }
.cache-key { font-family: 'JetBrains Mono', monospace; font-size: 11px; color: var(--ink-hint); background: var(--surface-2); padding: 2px 8px; border-radius: 5px; }

.cache-size-row { display: flex; align-items: baseline; gap: 14px; padding: 8px 0; border-bottom: 1.5px solid var(--border-light); }
.cache-size { display: flex; align-items: baseline; gap: 8px; }
.size-num { font-family: 'JetBrains Mono', monospace; font-size: 28px; font-weight: 800; color: var(--sun-deep); }
.size-label { font-size: 12px; font-weight: 600; color: var(--ink-soft); }
.cache-extra { font-size: 12px; color: var(--warn); font-weight: 600; }

.cache-desc { font-size: 12.5px; color: var(--ink-soft); line-height: 1.7; flex: 1; }
.cache-actions { display: flex; justify-content: flex-end; padding-top: 8px; border-top: 1.5px dashed var(--border-light); }

.btn { display: inline-flex; align-items: center; gap: 6px; padding: 7px 16px; border-radius: 9px; border: 1.5px solid var(--border); background: var(--surface); cursor: pointer; font-family: inherit; font-size: 13px; font-weight: 700; color: var(--ink-mid); transition: all .15s; }
.btn:hover { background: var(--sun-light); border-color: var(--peach); color: var(--sun-deep); }
.btn:disabled { opacity: .55; cursor: not-allowed; }
.btn-sm { padding: 6px 14px; font-size: 12.5px; }
.btn-danger { color: var(--err); border-color: color-mix(in srgb, var(--err) 30%, var(--border)); }
.btn-danger:hover { background: var(--err-bg); color: var(--err); border-color: var(--err); }

.empty-hint { text-align: center; color: var(--ink-hint); padding: 50px 20px; }
.empty-icon { font-size: 44px; margin-bottom: 12px; }
</style>
