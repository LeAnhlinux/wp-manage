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

// --- Threat actions: whitelist (mark as safe) + quarantine ---

export interface ThreatWhitelistEntry {
  id: number
  site_id: number
  file_path: string
  threat_type: string
  reason: string
  created_at: string
}

export interface QuarantinedFileEntry {
  id: number
  site_id: number
  original_path: string
  quarantine_path: string
  threat_type: string
  quarantined_at: string
}

export function useSiteWhitelist(siteId: number) {
  return useQuery({
    queryKey: ['threat-whitelist', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<ThreatWhitelistEntry[]>>(`/sites/${siteId}/threats/whitelist`)
      return data.data || []
    },
    enabled: !!siteId,
  })
}

export function useWhitelistThreat() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, file, threat_type, reason }: { siteId: number; file: string; threat_type?: string; reason?: string }) => {
      const { data } = await api.post<APIResponse<ThreatWhitelistEntry>>(`/sites/${siteId}/threats/whitelist`, { file, threat_type, reason })
      return data.data!
    },
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ['threat-whitelist', v.siteId] }),
  })
}

export function useRemoveWhitelist() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id }: { id: number; siteId: number }) => {
      await api.delete(`/threats/whitelist/${id}`)
    },
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ['threat-whitelist', v.siteId] }),
  })
}

export function useSiteQuarantine(siteId: number) {
  return useQuery({
    queryKey: ['threat-quarantine', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<QuarantinedFileEntry[]>>(`/sites/${siteId}/threats/quarantine`)
      return data.data || []
    },
    enabled: !!siteId,
  })
}

export function useQuarantineThreat() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, file, threat_type }: { siteId: number; file: string; threat_type?: string }) => {
      const { data } = await api.post<APIResponse<QuarantinedFileEntry>>(`/sites/${siteId}/threats/quarantine`, { file, threat_type })
      return data.data!
    },
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ['threat-quarantine', v.siteId] }),
  })
}

export function useRestoreQuarantine() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id }: { id: number; siteId: number }) => {
      await api.post(`/threats/quarantine/${id}/restore`)
    },
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ['threat-quarantine', v.siteId] }),
  })
}
