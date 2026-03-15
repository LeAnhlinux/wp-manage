import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { DomainEntry, CreateDomainRequest, UpdateDomainRequest, DNSCheckResult, APIResponse } from '@/types'

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

export function useUpdateDomain() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, domainId, ...req }: UpdateDomainRequest & { siteId: number; domainId: number }) => {
      const { data } = await api.put<APIResponse<DomainEntry>>(`/sites/${siteId}/domains/${domainId}`, req)
      return data.data!
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

// Issue SSL certificate for an alias domain (certbot expand). Runs synchronously — may take ~30-60s.
export function useIssueDomainSSL() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, domainId }: { siteId: number; domainId: number }) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/domains/${domainId}/ssl`, {})
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['domains', vars.siteId] })
    },
  })
}

// Check DNS resolution for a domain against the site's server IP.
export function useCheckDNS(siteId: number) {
  return useMutation({
    mutationFn: async (domain: string) => {
      const { data } = await api.get<APIResponse<DNSCheckResult>>(
        `/sites/${siteId}/domains/check-dns?domain=${encodeURIComponent(domain)}`
      )
      return data.data!
    },
  })
}
