import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { CronJob, CreateCronJobRequest, UpdateCronJobRequest, APIResponse } from '@/types'

export function useCronJobsByServer(serverId: number) {
  return useQuery({
    queryKey: ['cron-jobs', 'server', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<CronJob[]>>(`/servers/${serverId}/cron-jobs`)
      return data.data!
    },
    enabled: !!serverId,
  })
}

export function useCronJobsBySite(siteId: number) {
  return useQuery({
    queryKey: ['cron-jobs', 'site', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<CronJob[]>>(`/sites/${siteId}/cron-jobs`)
      return data.data!
    },
    enabled: !!siteId,
  })
}

export function useCreateCronJob() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ serverId, ...req }: CreateCronJobRequest & { serverId: number }) => {
      const { data } = await api.post<APIResponse<CronJob>>(`/servers/${serverId}/cron-jobs`, req)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['cron-jobs', 'server', vars.serverId] })
    },
  })
}

export function useUpdateCronJob() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ serverId, jobId, ...req }: UpdateCronJobRequest & { serverId: number; jobId: number }) => {
      const { data } = await api.put<APIResponse<CronJob>>(`/servers/${serverId}/cron-jobs/${jobId}`, req)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['cron-jobs', 'server', vars.serverId] })
    },
  })
}

export function useToggleCronJob() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ serverId, jobId }: { serverId: number; jobId: number }) => {
      const { data } = await api.post<APIResponse<CronJob>>(`/servers/${serverId}/cron-jobs/${jobId}/toggle`)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['cron-jobs', 'server', vars.serverId] })
    },
  })
}

export function useRunCronJob() {
  return useMutation({
    mutationFn: async ({ serverId, jobId }: { serverId: number; jobId: number }) => {
      const { data } = await api.post<APIResponse<{ output: string }>>(`/servers/${serverId}/cron-jobs/${jobId}/run`)
      return data
    },
  })
}

export function useCronJobLogs(serverId: number, jobId: number, lines = 100) {
  return useQuery({
    queryKey: ['cron-logs', serverId, jobId, lines],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<{ logs: string }>>(`/servers/${serverId}/cron-jobs/${jobId}/logs?lines=${lines}`)
      return data.data?.logs ?? ''
    },
    enabled: false, // only fetch on demand
  })
}

export function useDeleteCronJob() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ serverId, jobId }: { serverId: number; jobId: number }) => {
      const { data } = await api.delete<APIResponse>(`/servers/${serverId}/cron-jobs/${jobId}`)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['cron-jobs', 'server', vars.serverId] })
    },
  })
}
