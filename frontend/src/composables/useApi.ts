import { computed, type Ref } from 'vue'
import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query'
import api from '../api'

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

export function useOnlineUsers(enabled = true) {
  return useQuery({
    queryKey: ['onlineUsers'],
    queryFn: async () => {
      const res = await api.getOnlineUsers()
      return res.data
    },
    // `enabled` gates the initial fetch too — without this, a member's
    // Dashboard mount would still fire one GET /v1/auth/online that
    // 403s and spawns an error toast. With it, the query is dormant
    // until isAdmin flips true (which it won't for members).
    enabled,
    refetchInterval: enabled ? 30_000 : false,
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
