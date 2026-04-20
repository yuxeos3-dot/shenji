import { computed, unref, type Ref, type ComputedRef } from 'vue'
import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query'
import api from '../api'

type MaybeRef<T> = T | Ref<T> | ComputedRef<T>

export function useSites() {
  return useQuery({
    queryKey: ['sites'],
    queryFn: async () => {
      const res = await api.listSites()
      return res.data
    },
  })
}

export function useAudits(siteId: Ref<number | string>) {
  return useQuery({
    queryKey: ['audits', siteId],
    queryFn: async () => {
      const res = await api.listAudits(siteId.value)
      return res.data
    },
    enabled: computed(() => !!siteId.value),
  })
}

export function useOnlineUsers(enabled: MaybeRef<boolean> = true) {
  // Accept both plain boolean and reactive Ref<boolean>/ComputedRef<boolean>.
  // When a Ref is passed (e.g. `isAdmin` computed), the query reactively
  // enables/disables as the value changes — fixing the stale-snapshot bug
  // where passing `isAdmin.value` at mount time locked the query off.
  const enabledRef = computed(() => unref(enabled))

  return useQuery({
    queryKey: ['onlineUsers'],
    queryFn: async () => {
      const res = await api.getOnlineUsers()
      return res.data
    },
    enabled: enabledRef,
    refetchInterval: computed(() => enabledRef.value ? 30_000 : false),
  })
}

export function useTriggerAudit() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (siteId: number | string) => api.triggerAudit(siteId),
    onSuccess: (_data, siteId) => {
      qc.invalidateQueries({ queryKey: ['audits', siteId] })
    },
  })
}

export function useGSCData(siteId: Ref<number | string>, days = 30) {
  return useQuery({
    queryKey: ['gsc', siteId, days],
    queryFn: async () => {
      const res = await api.getGSCData(siteId.value, days)
      return res.data
    },
    enabled: computed(() => !!siteId.value),
  })
}

export function useGAData(siteId: Ref<number | string>) {
  return useQuery({
    queryKey: ['ga', siteId],
    queryFn: async () => {
      const res = await api.getGAData(siteId.value)
      return res.data
    },
    enabled: computed(() => !!siteId.value),
  })
}
