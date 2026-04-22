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
      // Invalidate so the new "running" record appears immediately
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
    // Poll every 2s while any run is in progress (agent sends progress every 50 images)
    refetchInterval: (query) => {
      const results = (query.state.data?.data as ImageOptResult[] | undefined) ?? []
      return results.some(r => r.status === 'running') ? 2000 : false
    },
  })
}

/** Returns the latest running result (if any) for the site — used to show live progress banner */
export function useRunningImageOpt(siteId: number) {
  const { data } = useImageOptResults(siteId, 1)
  const results = (data?.data as ImageOptResult[] | undefined) ?? []
  return results.find(r => r.status === 'running') ?? null
}

export function useToggleNginxWebP(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (enabled: boolean) => {
      const { data } = await api.post<APIResponse<ImageOptConfig>>(`/sites/${siteId}/image-optimization/nginx-webp`, { enabled })
      return data.data!
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['imageopt-config', siteId] })
    },
  })
}

export function useCancelImageOpt(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse<null>>(`/sites/${siteId}/image-optimization/cancel`)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['imageopt-results', siteId] })
    },
  })
}

export interface OriginalsStats {
  count: number
  size_bytes: number
}

export function useOriginalsStats(siteId: number) {
  return useQuery({
    queryKey: ['imageopt-originals-stats', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<OriginalsStats>>(`/sites/${siteId}/image-optimization/originals-stats`)
      return data.data!
    },
    staleTime: 30_000,
  })
}

export function useRevertImages(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse<OriginalsStats>>(`/sites/${siteId}/image-optimization/revert`)
      return data.data!
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['imageopt-originals-stats', siteId] })
      qc.invalidateQueries({ queryKey: ['imageopt-results', siteId] })
    },
  })
}

export function useDeleteOriginals(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse<OriginalsStats>>(`/sites/${siteId}/image-optimization/delete-originals`)
      return data.data!
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['imageopt-originals-stats', siteId] })
    },
  })
}
