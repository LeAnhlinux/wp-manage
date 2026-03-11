import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { Backup, BackupSchedule, CreateBackupRequest, RestoreRequest, CreateBackupScheduleRequest, PaginatedResponse, APIResponse } from '@/types'

// Poll every 3s while any backup is in "running" state, stop when all done.
const BACKUP_POLL_INTERVAL = 3000

function hasRunningBackup(data: PaginatedResponse<Backup> | undefined): boolean {
  return data?.data?.some(b => b.status === 'running') ?? false
}

export function useBackups(page = 1) {
  return useQuery({
    queryKey: ['backups', page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<Backup>>(`/backups?page=${page}`)
      return data
    },
    refetchInterval: (query) => hasRunningBackup(query.state.data) ? BACKUP_POLL_INTERVAL : false,
  })
}

// useServerBackups — list all backups on a server, including orphaned ones (site deleted).
export function useServerBackups(serverId: number, page = 1) {
  return useQuery({
    queryKey: ['server-backups', serverId, page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<Backup>>(`/servers/${serverId}/backups?page=${page}`)
      return data
    },
    enabled: !!serverId,
    refetchInterval: (query) => hasRunningBackup(query.state.data) ? BACKUP_POLL_INTERVAL : false,
  })
}

export function useSiteBackups(siteId: number, page = 1) {
  return useQuery({
    queryKey: ['site-backups', siteId, page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<Backup>>(`/sites/${siteId}/backups?page=${page}`)
      return data
    },
    enabled: !!siteId,
    refetchInterval: (query) => hasRunningBackup(query.state.data) ? BACKUP_POLL_INTERVAL : false,
  })
}

export function useBackup(id: number) {
  return useQuery({
    queryKey: ['backup', id],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<Backup>>(`/backups/${id}`)
      return data.data!
    },
    enabled: !!id,
  })
}

export function useCreateBackup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, ...req }: CreateBackupRequest & { siteId: number }) => {
      const { data } = await api.post<APIResponse<Backup>>(`/sites/${siteId}/backups`, req)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['backups'] })
      qc.invalidateQueries({ queryKey: ['site-backups'] })
      qc.invalidateQueries({ queryKey: ['server-backups'] })
    },
  })
}

export function useDeleteBackup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/backups/${id}`)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['backups'] })
      qc.invalidateQueries({ queryKey: ['site-backups'] })
      qc.invalidateQueries({ queryKey: ['server-backups'] })
    },
  })
}

export function useRestoreBackup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, ...req }: RestoreRequest & { siteId: number }) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/backups/restore`, req)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['sites'] })
    },
  })
}

// useRestoreBackupByID — restore directly by backup ID (works for orphaned backups too).
// Uses POST /backups/:id/restore instead of the site-scoped endpoint.
// serverId is optional — needed for legacy backups that were created before server_id was stored.
export function useRestoreBackupByID() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ backupId, confirmDomain, serverId }: { backupId: number; confirmDomain: string; serverId?: number }) => {
      const { data } = await api.post<APIResponse>(`/backups/${backupId}/restore`, {
        confirm_domain: confirmDomain,
        server_id: serverId || 0,
      })
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['backups'] })
      qc.invalidateQueries({ queryKey: ['site-backups'] })
      qc.invalidateQueries({ queryKey: ['server-backups'] })
      qc.invalidateQueries({ queryKey: ['sites'] })
    },
  })
}

// --- Backup Schedules ---

export function useSiteBackupSchedules(siteId: number) {
  return useQuery({
    queryKey: ['backup-schedules', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<BackupSchedule[]>>(`/sites/${siteId}/backup-schedules`)
      return data.data!
    },
    enabled: !!siteId,
  })
}

export function useCreateBackupSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, ...req }: CreateBackupScheduleRequest & { siteId: number }) => {
      const { data } = await api.post<APIResponse<BackupSchedule>>(`/sites/${siteId}/backup-schedules`, req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['backup-schedules'] }),
  })
}

export function useUpdateBackupSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, ...req }: { id: number } & Partial<CreateBackupScheduleRequest>) => {
      const { data } = await api.put<APIResponse<BackupSchedule>>(`/backup-schedules/${id}`, req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['backup-schedules'] }),
  })
}

export function useDeleteBackupSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/backup-schedules/${id}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['backup-schedules'] }),
  })
}
