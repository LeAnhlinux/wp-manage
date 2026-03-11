import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, PaginatedResponse, PerformanceMetric, RunPageSpeedRequest } from '@/types'

export function usePerformanceMetrics(siteId: number, deviceType?: string, days = 30, page = 1) {
  return useQuery({
    queryKey: ['performance', siteId, deviceType, days, page],
    queryFn: async () => {
      const params = new URLSearchParams({ days: String(days), page: String(page), per_page: '50' })
      if (deviceType) params.set('device_type', deviceType)
      const { data } = await api.get<PaginatedResponse<PerformanceMetric>>(`/sites/${siteId}/performance?${params}`)
      return data
    },
  })
}

export function useLatestPerformance(siteId: number) {
  return useQuery({
    queryKey: ['performance-latest', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<{ mobile: PerformanceMetric | null; desktop: PerformanceMetric | null }>>(`/sites/${siteId}/performance/latest`)
      return data.data!
    },
  })
}

export function useRunPageSpeed(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: RunPageSpeedRequest) => {
      const { data } = await api.post<APIResponse<PerformanceMetric>>(`/sites/${siteId}/performance/run`, req)
      return data.data!
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['performance', siteId] })
      qc.invalidateQueries({ queryKey: ['performance-latest', siteId] })
    },
  })
}
