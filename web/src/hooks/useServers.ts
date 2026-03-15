import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { Server, CreateServerRequest, AutoDeployRequest, PaginatedResponse, APIResponse } from '@/types'

export interface InstalledPHPVersion {
  version: string
  installed: boolean
  fpm_active: boolean
  opcache_enabled: boolean
}

export function useServers(page = 1) {
  return useQuery({
    queryKey: ['servers', page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<Server>>(`/servers?page=${page}`)
      return data
    },
  })
}

export function useServer(id: number) {
  return useQuery({
    queryKey: ['server', id],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<Server>>(`/servers/${id}`)
      return data.data!
    },
    enabled: !!id,
  })
}

export function useCreateServer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateServerRequest) => {
      const { data } = await api.post('/servers', req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['servers'] }),
  })
}

export function useDeleteServer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/servers/${id}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['servers'] }),
  })
}

export function useAutoDeploy() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: AutoDeployRequest) => {
      const { data } = await api.post<APIResponse<{ ws_channel: string }>>('/servers/deploy', req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['servers'] }),
  })
}

// Fetch list of PHP versions installed on a specific server.
// Uses existing GET /servers/:id/phpfpm/installed (synchronous agent task).
export function useServerInstalledPHP(serverId: number) {
  return useQuery({
    queryKey: ['server-php-versions', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<InstalledPHPVersion[]>>(`/servers/${serverId}/phpfpm/installed`)
      return data.data ?? []
    },
    enabled: !!serverId && serverId > 0,
    staleTime: 5 * 60 * 1000, // 5 minutes — PHP versions rarely change
    retry: false,
  })
}

// DNS check for site creation — uses serverID (no siteId yet).
// Call GET /servers/:id/check-dns?domain=...
export function useCheckDomainDNS() {
  return useMutation({
    mutationFn: async ({ serverId, domain }: { serverId: number; domain: string }) => {
      const { data } = await api.get<APIResponse<{
        domain: string
        server_ip: string
        resolved_ips: string[]
        pointing: boolean
        message: string
      }>>(`/servers/${serverId}/check-dns?domain=${encodeURIComponent(domain)}`)
      return data.data!
    },
  })
}
