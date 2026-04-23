import { ref, computed, Ref, watch } from 'vue'

/**
 * Client-side pagination composable.
 * Accepts a reactive source array (or ref), returns paged slice + controls.
 *
 * Usage:
 *   const { page, pageSize, total, paged } = usePagination(sites, 15)
 *   // template: <table v-for="row in paged">
 *   // template: <el-pagination :total="total" :page-size="pageSize" v-model:current-page="page" />
 */
export function usePagination<T>(source: Ref<T[]> | (() => T[]), size = 15) {
  const page = ref(1)
  const pageSize = ref(size)

  const items = computed(() => {
    return typeof source === 'function' ? source() : source.value
  })

  const total = computed(() => items.value?.length || 0)

  const paged = computed(() => {
    const start = (page.value - 1) * pageSize.value
    return (items.value || []).slice(start, start + pageSize.value)
  })

  // Reset to page 1 when source changes (avoid showing empty page after filter)
  watch(total, (newTotal) => {
    const maxPage = Math.max(1, Math.ceil(newTotal / pageSize.value))
    if (page.value > maxPage) page.value = maxPage
  })

  return { page, pageSize, total, paged }
}
