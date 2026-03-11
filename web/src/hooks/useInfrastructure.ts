import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type {
  FirewallRule,
  CreateFirewallRuleRequest,
  NginxConfig,
  CreateNginxConfigRequest,
  UpdateNginxConfigRequest,
  OPcacheActionRequest,
  RebuildConfigRequest,
  APIResponse,
  VhostData,
  PHPFPMPoolConfig,
  MariaDBUser,
  MariaDBUserRequest,
  MariaDBConfig,
  MariaDBDatabase,
  CreateDatabaseRequest,
  InstalledPHPVersion,
} from '@/types'

// ==================== Firewall ====================

export function useFirewallRules(serverId: number) {
  return useQuery({
    queryKey: ['firewall-rules', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<FirewallRule[]>>(`/servers/${serverId}/firewall/rules`)
      return data.data!
    },
    enabled: !!serverId,
  })
}

export function useCreateFirewallRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ serverId, ...req }: CreateFirewallRuleRequest & { serverId: number }) => {
      const { data } = await api.post<APIResponse<FirewallRule>>(`/servers/${serverId}/firewall/rules`, req)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['firewall-rules', vars.serverId] })
    },
  })
}

export function useDeleteFirewallRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ serverId, ruleId }: { serverId: number; ruleId: number }) => {
      const { data } = await api.delete<APIResponse>(`/servers/${serverId}/firewall/rules/${ruleId}`)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['firewall-rules', vars.serverId] })
    },
  })
}

// ==================== Nginx Configs ====================

export function useNginxConfigs(serverId: number) {
  return useQuery({
    queryKey: ['nginx-configs', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<NginxConfig[]>>(`/servers/${serverId}/nginx-configs`)
      return data.data!
    },
    enabled: !!serverId,
  })
}

export function useCreateNginxConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ serverId, ...req }: CreateNginxConfigRequest & { serverId: number }) => {
      const { data } = await api.post<APIResponse<NginxConfig>>(`/servers/${serverId}/nginx-configs`, req)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['nginx-configs', vars.serverId] })
    },
  })
}

export function useUpdateNginxConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ serverId, configId, ...req }: UpdateNginxConfigRequest & { serverId: number; configId: number }) => {
      const { data } = await api.put<APIResponse<NginxConfig>>(`/servers/${serverId}/nginx-configs/${configId}`, req)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['nginx-configs', vars.serverId] })
    },
  })
}

export function useDeleteNginxConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ serverId, configId }: { serverId: number; configId: number }) => {
      const { data } = await api.delete<APIResponse>(`/servers/${serverId}/nginx-configs/${configId}`)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['nginx-configs', vars.serverId] })
    },
  })
}

// ==================== Nginx Vhost (per-site) ====================

export function useReadVhost(siteId: number) {
  return useQuery({
    queryKey: ['vhost', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<VhostData>>(`/sites/${siteId}/nginx/vhost`)
      return data.data!
    },
    enabled: false, // Manual fetch only
  })
}

export function useWriteVhost(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (content: string) => {
      const { data } = await api.put<APIResponse>(`/sites/${siteId}/nginx/vhost`, { content })
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['vhost', siteId] })
    },
  })
}

// ==================== PHP-FPM Pool (per-site) ====================

export function usePHPFPMPool(serverId: number, siteId: number) {
  return useQuery({
    queryKey: ['phpfpm-pool', serverId, siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<PHPFPMPoolConfig>>(`/servers/${serverId}/phpfpm/${siteId}`)
      return data.data!
    },
    enabled: !!serverId && !!siteId,
  })
}

export function useUpdatePHPFPMPool(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, ...config }: PHPFPMPoolConfig & { siteId: number }) => {
      const { data } = await api.put<APIResponse>(`/servers/${serverId}/phpfpm/${siteId}`, config)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['phpfpm-pool', serverId, vars.siteId] })
    },
  })
}

// ==================== OPcache ====================

export function useOPcacheAction() {
  return useMutation({
    mutationFn: async ({ serverId, action, phpVersion }: { serverId: number; action: string; phpVersion?: string }) => {
      const params = phpVersion ? `?php_version=${phpVersion}` : ''
      const { data } = await api.post<APIResponse>(`/servers/${serverId}/opcache${params}`, { action })
      return data
    },
  })
}

// ==================== PHP Version Management ====================

export function useInstallPHPVersion() {
  return useMutation({
    mutationFn: async ({ serverId, version }: { serverId: number; version: string }) => {
      const { data } = await api.post<APIResponse>(`/servers/${serverId}/phpfpm/install`, { version })
      return data
    },
  })
}

export function useChangeSitePHPVersion(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, newVersion }: { siteId: number; newVersion: string }) => {
      const { data } = await api.put<APIResponse>(`/servers/${serverId}/phpfpm/${siteId}/version`, { new_version: newVersion })
      return data
    },
    onSuccess: () => {
      // Invalidate server + site data so php_version field refreshes
      qc.invalidateQueries({ queryKey: ['server', serverId] })
      qc.invalidateQueries({ queryKey: ['sites'] })
    },
  })
}

// ==================== MariaDB Management ====================

export function useMariaDBUsers(serverId: number) {
  return useQuery({
    queryKey: ['mariadb-users', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<MariaDBUser[]>>(`/servers/${serverId}/mariadb/users`)
      return data.data ?? []
    },
    enabled: !!serverId,
  })
}

export function useCreateMariaDBUser(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: MariaDBUserRequest) => {
      const { data } = await api.post<APIResponse>(`/servers/${serverId}/mariadb/users`, req)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['mariadb-users', serverId] })
    },
  })
}

export function useDeleteMariaDBUser(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (username: string) => {
      const { data } = await api.delete<APIResponse>(`/servers/${serverId}/mariadb/users/${username}`)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['mariadb-users', serverId] })
    },
  })
}

export function useMariaDBConfig(serverId: number) {
  return useQuery({
    queryKey: ['mariadb-config', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<MariaDBConfig>>(`/servers/${serverId}/mariadb/config`)
      return data.data!
    },
    enabled: !!serverId,
  })
}

export function useUpdateMariaDBConfig(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (cfg: MariaDBConfig) => {
      const { data } = await api.put<APIResponse>(`/servers/${serverId}/mariadb/config`, cfg)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['mariadb-config', serverId] })
    },
  })
}

// ==================== MariaDB Databases ====================

export function useMariaDBDatabases(serverId: number) {
  return useQuery({
    queryKey: ['mariadb-databases', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<MariaDBDatabase[]>>(`/servers/${serverId}/mariadb/databases`)
      return data.data ?? []
    },
    enabled: !!serverId,
  })
}

export function useCreateMariaDBDatabase(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateDatabaseRequest) => {
      const { data } = await api.post<APIResponse>(`/servers/${serverId}/mariadb/databases`, req)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['mariadb-databases', serverId] })
    },
  })
}

export function useDeleteMariaDBDatabase(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (dbName: string) => {
      const { data } = await api.delete<APIResponse>(`/servers/${serverId}/mariadb/databases/${dbName}`)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['mariadb-databases', serverId] })
    },
  })
}

export function useImportMariaDBDatabase(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ dbName, file }: { dbName: string; file: File }) => {
      const formData = new FormData()
      formData.append('file', file)
      const { data } = await api.post<APIResponse>(
        `/servers/${serverId}/mariadb/databases/${encodeURIComponent(dbName)}/import`,
        formData,
        { headers: { 'Content-Type': 'multipart/form-data' } }
      )
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['mariadb-databases', serverId] })
    },
  })
}

// ==================== Installed PHP Versions ====================

export function useInstalledPHPVersions(serverId: number) {
  return useQuery({
    queryKey: ['installed-php', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<InstalledPHPVersion[]>>(`/servers/${serverId}/phpfpm/installed`)
      return data.data ?? []
    },
    enabled: !!serverId,
  })
}

// ==================== Rebuild Config ====================

export function useRebuildConfig() {
  return useMutation({
    mutationFn: async ({ serverId, ...req }: RebuildConfigRequest & { serverId: number }) => {
      const { data } = await api.post<APIResponse<{ output: string }>>(`/servers/${serverId}/rebuild-config`, req)
      return data
    },
  })
}

// ==================== Health Check ====================

export interface HealthCheckResult {
  config_type: string
  file_path?: string
  status: 'ok' | 'warning' | 'error'
  message: string
  diff?: string
}

export function useRunHealthCheck() {
  return useMutation({
    mutationFn: async (serverId: number) => {
      const { data } = await api.post<APIResponse<{ results: string }>>(`/servers/${serverId}/health-check`)
      // Parse the JSON string from results
      const raw = data.data?.results ?? '[]'
      try {
        return JSON.parse(raw) as HealthCheckResult[]
      } catch {
        return [] as HealthCheckResult[]
      }
    },
  })
}

// ==================== Server Logs ====================

export function useServerLogs() {
  return useMutation({
    mutationFn: async ({ serverId, logType, lines }: { serverId: number; logType: string; lines: number }) => {
      const { data } = await api.get<APIResponse<{ content: string; log_type: string; lines: number }>>(
        `/servers/${serverId}/logs?type=${logType}&lines=${lines}`
      )
      return data.data?.content ?? ''
    },
  })
}


// ==================== Firewall Status ====================

export function useFirewallStatus() {
  return useMutation({
    mutationFn: async (serverId: number) => {
      const { data } = await api.get<APIResponse<{ status: string }>>(`/servers/${serverId}/firewall/status`)
      return data.data?.status ?? ''
    },
  })
}
