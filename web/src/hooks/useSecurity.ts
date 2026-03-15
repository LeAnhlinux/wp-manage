import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { WAFConfig, UpdateWAFRequest, MalwareScan, MalwareScanSchedule, StartMalwareScanRequest, PaginatedResponse, APIResponse, ClamAVStatus } from '@/types'

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
    // Auto-refresh every 3s while any scan is running
    refetchInterval: (query) => {
      const scans: MalwareScan[] = query.state.data?.data || []
      return scans.some(s => s.status === 'running') ? 3000 : false
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

// --- ClamAV ---

export function useClamAVStatus(serverId: number) {
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.get<APIResponse<ClamAVStatus>>(`/servers/${serverId}/clamav/status`)
      return data.data!
    },
  })
}

export function useInstallClamAV(serverId: number) {
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse<null>>(`/servers/${serverId}/clamav/install`, {})
      return data
    },
  })
}

// --- Scan Schedules ---

export function useScanSchedules(siteId: number) {
  return useQuery({
    queryKey: ['scan-schedules', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<MalwareScanSchedule[]>>(`/sites/${siteId}/scans/schedule`)
      return data.data || []
    },
    enabled: !!siteId,
  })
}

export function useCreateScanSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, ...req }: { siteId: number; scan_type: string; engine: string; interval_h: number }) => {
      const { data } = await api.post<APIResponse<MalwareScanSchedule>>(`/sites/${siteId}/scans/schedule`, req)
      return data.data!
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['scan-schedules', vars.siteId] })
      qc.invalidateQueries({ queryKey: ['scan-schedules-all'] })
    },
  })
}

export function useUpdateScanSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, id, ...req }: { siteId: number; id: number; scan_type?: string; engine?: string; interval_h?: number; enabled?: boolean }) => {
      const { data } = await api.put<APIResponse<MalwareScanSchedule>>(`/sites/${siteId}/scans/schedule/${id}`, req)
      return data.data!
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['scan-schedules', vars.siteId] })
      qc.invalidateQueries({ queryKey: ['scan-schedules-all'] })
    },
  })
}

export function useDeleteScanSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, id }: { siteId: number; id: number }) => {
      await api.delete(`/sites/${siteId}/scans/schedule/${id}`)
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['scan-schedules', vars.siteId] })
      qc.invalidateQueries({ queryKey: ['scan-schedules-all'] })
    },
  })
}
