import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, UptimeCheck, UptimeLog, CreateUptimeCheckRequest } from '@/types'

export function useUptimeChecks(siteId: number) {
  return useQuery({
    queryKey: ['uptime', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<UptimeCheck[]>>(`/sites/${siteId}/uptime`)
      return data.data || []
    },
  })
}

export function useCreateUptimeCheck(siteId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateUptimeCheckRequest) => {
      const { data } = await api.post<APIResponse<UptimeCheck>>(`/sites/${siteId}/uptime`, req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['uptime', siteId] })
    },
  })
}

export function useUpdateUptimeCheck(siteId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ checkId, ...req }: { checkId: number; url?: string; interval?: number; enabled?: boolean }) => {
      const { data } = await api.put<APIResponse<UptimeCheck>>(`/sites/${siteId}/uptime/${checkId}`, req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['uptime', siteId] })
    },
  })
}

export function useDeleteUptimeCheck(siteId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (checkId: number) => {
      await api.delete(`/sites/${siteId}/uptime/${checkId}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['uptime', siteId] })
    },
  })
}

export function useUptimeHistory(siteId: number, checkId: number) {
  return useQuery({
    queryKey: ['uptime-history', siteId, checkId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<UptimeLog[]>>(`/sites/${siteId}/uptime/${checkId}/history?limit=100`)
      return data.data || []
    },
    enabled: checkId > 0,
  })
}
