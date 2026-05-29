import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { ResourceLimits, UpdateResourceLimitsRequest, APIResponse } from '@/types'

export function useResourceLimits(siteId: number) {
  return useQuery({
    queryKey: ['resource-limits', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<ResourceLimits>>(`/sites/${siteId}/limits`)
      return data.data!
    },
    enabled: !!siteId,
  })
}

export function useUpdateResourceLimits(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: UpdateResourceLimitsRequest) => {
      const { data } = await api.put<APIResponse>(`/sites/${siteId}/limits`, req)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['resource-limits', siteId] })
      qc.invalidateQueries({ queryKey: ['site', siteId] })
    },
  })
}

export function useScanDisk(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/disk/scan`)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['resource-limits', siteId] })
    },
  })
}
