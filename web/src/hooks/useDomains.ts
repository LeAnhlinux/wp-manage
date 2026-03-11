import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { DomainEntry, CreateDomainRequest, APIResponse } from '@/types'

export function useDomains(siteId: number) {
  return useQuery({
    queryKey: ['domains', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<DomainEntry[]>>(`/sites/${siteId}/domains`)
      return data.data!
    },
    enabled: !!siteId,
  })
}

export function useCreateDomain() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, ...req }: CreateDomainRequest & { siteId: number }) => {
      const { data } = await api.post<APIResponse<DomainEntry>>(`/sites/${siteId}/domains`, req)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['domains', vars.siteId] })
    },
  })
}

export function useDeleteDomain() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, domainId }: { siteId: number; domainId: number }) => {
      const { data } = await api.delete<APIResponse>(`/sites/${siteId}/domains/${domainId}`)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['domains', vars.siteId] })
    },
  })
}
