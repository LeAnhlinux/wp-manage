import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, PaginatedResponse, StagingSite, AutoUpdateConfig, UpdateAutoUpdateConfigRequest, UpdateLog } from '@/types'

// --- Staging ---

export function useStagingSites(siteId: number) {
  return useQuery({
    queryKey: ['staging', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<StagingSite[]>>(`/sites/${siteId}/staging`)
      return data
    },
    enabled: siteId > 0,
  })
}

export function useCreateStaging() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (siteId: number) => {
      const { data } = await api.post<APIResponse<StagingSite>>(`/sites/${siteId}/staging`)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['staging'] }),
  })
}

export function usePushStaging() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (stagingId: number) => {
      const { data } = await api.post<APIResponse>(`/staging/${stagingId}/push`)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['staging'] }),
  })
}

export function useDeleteStaging() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (stagingId: number) => {
      const { data } = await api.delete<APIResponse>(`/staging/${stagingId}`)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['staging'] }),
  })
}

// --- Auto-Update Config ---

export function useAutoUpdateConfig(siteId: number) {
  return useQuery({
    queryKey: ['auto-update', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<AutoUpdateConfig>>(`/sites/${siteId}/auto-update`)
      return data
    },
    enabled: siteId > 0,
  })
}

export function useUpdateAutoUpdateConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, ...req }: UpdateAutoUpdateConfigRequest & { siteId: number }) => {
      const { data } = await api.put<APIResponse<AutoUpdateConfig>>(`/sites/${siteId}/auto-update`, req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['auto-update'] }),
  })
}

// --- Check Updates ---

export function useCheckUpdates() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (siteId: number) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/updates/check`)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['update-logs'] }),
  })
}

export function useUpdateLogs(siteId: number, page = 1) {
  return useQuery({
    queryKey: ['update-logs', siteId, page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<UpdateLog>>(`/sites/${siteId}/updates/logs?page=${page}&per_page=20`)
      return data
    },
    enabled: siteId > 0,
  })
}

// --- DB Optimize ---

export function useOptimizeDatabase() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (siteId: number) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/db/optimize`)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['update-logs'] }),
  })
}
