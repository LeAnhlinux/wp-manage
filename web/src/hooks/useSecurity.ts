import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { WAFConfig, UpdateWAFRequest, MalwareScan, StartMalwareScanRequest, PaginatedResponse, APIResponse } from '@/types'

// --- WAF ---

export function useWAFConfig(siteId: number) {
  return useQuery({
    queryKey: ['waf-config', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<WAFConfig>>(`/sites/${siteId}/waf`)
      return data.data!
    },
    enabled: !!siteId,
  })
}

export function useUpdateWAFConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, ...req }: UpdateWAFRequest & { siteId: number }) => {
      const { data } = await api.put<APIResponse<WAFConfig>>(`/sites/${siteId}/waf`, req)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['waf-config', vars.siteId] })
    },
  })
}

// --- Malware Scans ---

export function useMalwareScans(page = 1) {
  return useQuery({
    queryKey: ['malware-scans', page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<MalwareScan>>(`/scans?page=${page}`)
      return data
    },
  })
}

export function useSiteMalwareScans(siteId: number, page = 1) {
  return useQuery({
    queryKey: ['site-malware-scans', siteId, page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<MalwareScan>>(`/sites/${siteId}/scans?page=${page}`)
      return data
    },
    enabled: !!siteId,
  })
}

export function useMalwareScan(id: number) {
  return useQuery({
    queryKey: ['malware-scan', id],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<MalwareScan>>(`/scans/${id}`)
      return data.data!
    },
    enabled: !!id,
    refetchInterval: (query) => {
      const scan = query.state.data
      if (scan && scan.status === 'running') return 5000
      return false
    },
  })
}

export function useStartMalwareScan() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, ...req }: StartMalwareScanRequest & { siteId: number }) => {
      const { data } = await api.post<APIResponse<MalwareScan>>(`/sites/${siteId}/scans`, req)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['malware-scans'] })
      qc.invalidateQueries({ queryKey: ['site-malware-scans'] })
    },
  })
}
