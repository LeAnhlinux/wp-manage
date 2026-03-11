import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, CacheConfig, UpdateCacheConfigRequest } from '@/types'

export function useCacheConfig(siteId: number) {
  return useQuery({
    queryKey: ['cache-config', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<CacheConfig>>(`/sites/${siteId}/cache`)
      return data.data!
    },
    enabled: !!siteId,
  })
}

export function useUpdateCacheConfig(siteId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: UpdateCacheConfigRequest) => {
      const { data } = await api.put<APIResponse<CacheConfig>>(`/sites/${siteId}/cache`, req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cache-config', siteId] })
    },
  })
}

export function useToggleCache(siteId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (enabled: boolean) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/cache/toggle`, { enabled })
      return data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cache-config', siteId] })
    },
  })
}

export function usePurgeCache(siteId: number) {
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/cache/purge`)
      return data
    },
  })
}
