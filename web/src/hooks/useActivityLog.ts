import { useQuery } from '@tanstack/react-query'
import api from '@/lib/api'
import type { PaginatedResponse, ActivityLog, APIResponse } from '@/types'

export function useActivityLogs(
  page = 1,
  perPage = 20,
  filters?: {
    user_id?: number
    server_id?: number
    site_id?: number
    action?: string
    from?: string
    to?: string
  }
) {
  return useQuery({
    queryKey: ['activity-logs', page, perPage, filters],
    queryFn: async () => {
      const params = new URLSearchParams({
        page: String(page),
        per_page: String(perPage),
      })
      if (filters?.user_id) params.set('user_id', String(filters.user_id))
      if (filters?.server_id) params.set('server_id', String(filters.server_id))
      if (filters?.site_id) params.set('site_id', String(filters.site_id))
      if (filters?.action) params.set('action', filters.action)
      if (filters?.from) params.set('from', filters.from)
      if (filters?.to) params.set('to', filters.to)

      const { data } = await api.get<PaginatedResponse<ActivityLog>>(`/activity-logs?${params}`)
      return data
    },
  })
}

export function useActivityActions() {
  return useQuery({
    queryKey: ['activity-actions'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<string[]>>('/activity-logs/actions')
      return data.data || []
    },
  })
}
