import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, PaginatedResponse, ImageOptConfig, ImageOptResult, UpdateImageOptConfigRequest } from '@/types'

export function useImageOptConfig(siteId: number) {
  return useQuery({
    queryKey: ['imageopt-config', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<ImageOptConfig>>(`/sites/${siteId}/image-optimization`)
      return data.data!
    },
  })
}

export function useUpdateImageOptConfig(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: UpdateImageOptConfigRequest) => {
      const { data } = await api.put<APIResponse<ImageOptConfig>>(`/sites/${siteId}/image-optimization`, req)
      return data.data!
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['imageopt-config', siteId] })
    },
  })
}

export function useToggleImageOpt(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (enabled: boolean) => {
      const { data } = await api.post<APIResponse<ImageOptConfig>>(`/sites/${siteId}/image-optimization/toggle`, { enabled })
      return data.data!
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['imageopt-config', siteId] })
    },
  })
}

export function useRunImageOpt(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse<ImageOptResult>>(`/sites/${siteId}/image-optimization/run`)
      return data.data!
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['imageopt-results', siteId] })
    },
  })
}

export function useImageOptResults(siteId: number, page = 1) {
  return useQuery({
    queryKey: ['imageopt-results', siteId, page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<ImageOptResult[]>>(`/sites/${siteId}/image-optimization/results?page=${page}&per_page=10`)
      return data
    },
    // Auto-refresh while any run is in progress
    refetchInterval: (query) => {
      const results = (query.state.data?.data as ImageOptResult[] | undefined) ?? []
      return results.some(r => r.status === 'running') ? 3000 : false
    },
  })
}
