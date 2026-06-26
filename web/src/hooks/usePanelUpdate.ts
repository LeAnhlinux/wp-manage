import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, PanelUpdateStatus, PanelRelease, CreatePanelReleaseRequest } from '@/types'

export function usePanelUpdateStatus() {
  return useQuery({
    queryKey: ['panel-update'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<PanelUpdateStatus>>('/settings/panel-update')
      return data.data!
    },
  })
}

export function usePanelReleases() {
  return useQuery({
    queryKey: ['panel-releases'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<PanelRelease[]>>('/settings/panel-update/releases')
      return data.data || []
    },
  })
}

export function useCreatePanelRelease() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreatePanelReleaseRequest) => {
      const { data } = await api.post<APIResponse<PanelRelease>>('/settings/panel-update/releases', req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['panel-releases'] })
      queryClient.invalidateQueries({ queryKey: ['panel-update'] })
    },
  })
}

export function useApplyRelease() {
  return useMutation({
    mutationFn: async (releaseId: number) => {
      // Returns 202 — API will restart shortly after
      await api.post(`/settings/panel-update/releases/${releaseId}/apply`)
    },
  })
}

// --- Simple auto-update (check mirror + one-click apply) ---

export interface ServerAgentInfo {
  id: number
  name: string
  ip_address: string
  agent_version: string
  needs_update: boolean
}

export interface CheckUpdateResult {
  current_panel: string
  current_agent: string
  latest: {
    version: string
    released_at: string
    release_notes: string
    panel: { download_url: string; checksum: string }
    agent: { download_url: string; checksum: string }
    web: { download_url: string; checksum: string }
  } | null
  panel_newer: boolean
  agent_newer: boolean
  server_agents: ServerAgentInfo[]
}

export function useCheckUpdate() {
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.get<APIResponse<CheckUpdateResult>>('/settings/update/check')
      return data.data!
    },
  })
}

// Background query — checks mirror once per hour silently.
// Returns { hasUpdate, latestVersion } for the sidebar badge.
export function useUpdateAvailable() {
  return useQuery({
    queryKey: ['panel-update-available'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<CheckUpdateResult>>('/settings/update/check')
      const result = data.data
      return {
        hasUpdate: !!(result?.panel_newer || result?.agent_newer),
        panelNewer: !!result?.panel_newer,
        agentNewer: !!result?.agent_newer,
        latestVersion: result?.latest?.version ?? null,
      }
    },
    // Cache 30 min — recheck every 1h (only when tab is focused)
    staleTime: 30 * 60 * 1000,
    refetchInterval: 60 * 60 * 1000,
    refetchIntervalInBackground: false,
    // Don't throw — silent failure is fine for a background badge
    retry: 1,
  })
}

export function useApplyPanelUpdate() {
  return useMutation({
    mutationFn: async (req: { version: string; download_url: string; checksum: string; web_url?: string; web_checksum?: string }) => {
      await api.post('/settings/update/apply-panel', req)
    },
  })
}

export function useApplyAgentUpdate() {
  return useMutation({
    mutationFn: async (req: { version: string; download_url: string; checksum: string; server_ids?: number[] }) => {
      const { data } = await api.post<APIResponse<{ rollout_id: number }>>('/settings/update/apply-agents', req)
      return data.data!
    },
  })
}

export interface UpdateResult {
  status: 'completed' | 'rolled_back' | 'failed'
  new_version?: string
  old_version?: string
  error?: string
  timestamp?: string
}

export function useLastUpdateResult() {
  return useQuery({
    queryKey: ['panel-last-result'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<UpdateResult>>('/settings/panel-update/last-result')
      return data.data || null
    },
    retry: false,
  })
}
