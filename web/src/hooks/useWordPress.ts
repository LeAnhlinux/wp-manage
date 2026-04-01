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
    // Poll every 3s when any staging is in a transient state (creating/pushing)
    refetchInterval: (query) => {
      const stagings = query.state.data?.data ?? []
      const hasTransient = stagings.some((s: StagingSite) => s.status === 'creating' || s.status === 'pushing')
      return hasTransient ? 3000 : false
    },
  })
}

export function useCheckDNSProvider(domain: string) {
  return useQuery({
    queryKey: ['dns-provider-check', domain],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<{ found: boolean; provider_type: string; provider_name: string }>>(`/dns-providers/check?domain=${encodeURIComponent(domain)}`)
      return data
    },
    enabled: domain.length > 3,
    staleTime: 30000,
  })
}

export function useCreateStaging() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, stagingDomain }: { siteId: number; stagingDomain?: string }) => {
      const { data } = await api.post<APIResponse<StagingSite>>(`/sites/${siteId}/staging`, { staging_domain: stagingDomain || '' })
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

export interface CheckUpdatesResult {
  message?: string
  core?: UpdateLog | null
  plugins?: UpdateLog[]
  themes?: UpdateLog[]
}

export function useCheckUpdates() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (siteId: number) => {
      const { data } = await api.post<APIResponse<CheckUpdatesResult>>(`/sites/${siteId}/updates/check`)
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

export interface DBOptimizeResult {
  status: string
  tables_optimized: number
  space_saved: number       // bytes
  revisions_cleaned: number
  transients_cleared: number
  error?: string
  created_at: string
}

export function useOptimizeDatabase() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (siteId: number) => {
      const { data } = await api.post<APIResponse<DBOptimizeResult>>(`/sites/${siteId}/db/optimize`)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['update-logs'] }),
  })
}
