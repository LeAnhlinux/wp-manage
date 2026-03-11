import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, AlertRule, AlertHistory, CreateAlertRuleRequest, NotificationConfig } from '@/types'

export function useAlertRules(serverId: number) {
  return useQuery({
    queryKey: ['alerts', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<AlertRule[]>>(`/servers/${serverId}/alerts`)
      return data.data || []
    },
  })
}

export function useCreateAlertRule(serverId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateAlertRuleRequest) => {
      const { data } = await api.post<APIResponse<AlertRule>>(`/servers/${serverId}/alerts`, req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['alerts', serverId] })
    },
  })
}

export function useUpdateAlertRule(serverId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ ruleId, ...req }: { ruleId: number; metric?: string; threshold?: number; channel?: string; enabled?: boolean; cooldown_min?: number }) => {
      const { data } = await api.put<APIResponse<AlertRule>>(`/servers/${serverId}/alerts/${ruleId}`, req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['alerts', serverId] })
    },
  })
}

export function useDeleteAlertRule(serverId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (ruleId: number) => {
      await api.delete(`/servers/${serverId}/alerts/${ruleId}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['alerts', serverId] })
    },
  })
}

export function useAlertHistory(serverId: number) {
  return useQuery({
    queryKey: ['alert-history', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<AlertHistory[]>>(`/servers/${serverId}/alerts/history?limit=50`)
      return data.data || []
    },
  })
}

export function useNotificationConfig() {
  return useQuery({
    queryKey: ['notification-config'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<NotificationConfig>>('/settings/notifications')
      return data.data!
    },
  })
}

export function useUpdateNotificationConfig() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: Partial<NotificationConfig & { smtp_pass: string }>) => {
      const { data } = await api.put<APIResponse<NotificationConfig>>('/settings/notifications', req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notification-config'] })
    },
  })
}

export function usePHPFPMAutotune(serverId: number) {
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse>(`/servers/${serverId}/phpfpm-autotune`)
      return data
    },
  })
}
