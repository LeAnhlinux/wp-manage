import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { Migration, MigrationLog, MigrateRequest, PaginatedResponse, APIResponse } from '@/types'

export function useMigrations(page = 1) {
  return useQuery({
    queryKey: ['migrations', page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<Migration>>(`/migrations?page=${page}`)
      return data
    },
  })
}

export function useMigration(id: number) {
  return useQuery({
    queryKey: ['migration', id],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<Migration>>(`/migrations/${id}`)
      return data.data!
    },
    enabled: !!id,
    refetchInterval: (query) => {
      // Auto-refresh while migration is running
      const migration = query.state.data
      if (migration && migration.status === 'running') return 3000
      return false
    },
  })
}

export function useMigrationLogs(id: number) {
  return useQuery({
    queryKey: ['migration-logs', id],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<MigrationLog[]>>(`/migrations/${id}/logs`)
      return data.data!
    },
    enabled: !!id,
    refetchInterval: 5000, // refresh logs every 5s
  })
}

export function useStartMigration() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, ...req }: MigrateRequest & { siteId: number }) => {
      const { data } = await api.post<APIResponse<Migration>>(`/sites/${siteId}/migrate`, req)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['migrations'] })
      qc.invalidateQueries({ queryKey: ['sites'] })
    },
  })
}

export function useRollbackMigration() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { data } = await api.post<APIResponse>(`/migrations/${id}/rollback`)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['migrations'] })
      qc.invalidateQueries({ queryKey: ['sites'] })
    },
  })
}

export function useCancelMigration() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { data } = await api.post<APIResponse>(`/migrations/${id}/cancel`)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['migrations'] })
    },
  })
}
