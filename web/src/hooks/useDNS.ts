import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, DNSProvider, CreateDNSProviderRequest, DNSRecord, CreateDNSRecordRequest } from '@/types'

export interface TestDNSProviderResult {
  success: boolean
  message: string
  zone_name?: string
}

export function useTestDNSProvider() {
  return useMutation({
    mutationFn: async (req: import('@/types').CreateDNSProviderRequest) => {
      const { data } = await api.post<import('@/types').APIResponse<TestDNSProviderResult>>('/dns-providers/test', req)
      return data.data!
    },
  })
}

// ─── Global (Option B) hooks ────────────────────────────────────────────────

export interface LookupZoneResult {
  zone_id: string
  zone_name: string
}

export function useLookupZone() {
  return useMutation({
    mutationFn: async (req: { provider_type: string; api_key: string; api_email?: string; domain: string }) => {
      const { data } = await api.post<APIResponse<LookupZoneResult>>('/dns-providers/lookup-zone', req)
      return data.data!
    },
  })
}

export interface AutoCreateDNSRecordResult {
  record_id: string
  provider_id: number
  domain: string
  ip: string
  action: 'created' | 'updated'
  zone_name: string
  proxied: boolean // true = Cloudflare proxy still on → DNS check will show CF IPs, not server IP
}

export function useAutoCreateDNSRecord() {
  return useMutation({
    mutationFn: async (req: { domain: string; server_ip: string }) => {
      const { data } = await api.post<APIResponse<AutoCreateDNSRecordResult>>('/dns-providers/auto-create-record', req)
      return data.data!
    },
  })
}

export function useCreateGlobalDNSProvider() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: { provider_type: string; api_key: string; api_email?: string; zone_id: string; domain: string }) => {
      const { data } = await api.post<APIResponse<DNSProvider>>('/dns-providers', req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns-providers', 'all'] })
    },
  })
}

export function useDeleteGlobalDNSProvider() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (providerId: number) => {
      await api.delete(`/dns-providers/${providerId}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns-providers', 'all'] })
    },
  })
}

export function useGlobalDNSRecords(providerId: number | null) {
  return useQuery({
    queryKey: ['dns-records-global', providerId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<DNSRecord[]>>(`/dns-providers/${providerId}/records`)
      return data.data || []
    },
    enabled: !!providerId,
  })
}

export function useCreateGlobalDNSRecord(providerId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateDNSRecordRequest) => {
      const { data } = await api.post<APIResponse<DNSRecord>>(`/dns-providers/${providerId}/records`, req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns-records-global', providerId] })
    },
  })
}

export function useDeleteGlobalDNSRecord(providerId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (recordId: string) => {
      await api.delete(`/dns-providers/${providerId}/records/${recordId}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns-records-global', providerId] })
    },
  })
}

export function useUpdateGlobalDNSRecord(providerId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ recordId, ...rec }: CreateDNSRecordRequest & { recordId: string }) => {
      const { data } = await api.put<APIResponse<DNSRecord>>(`/dns-providers/${providerId}/records/${recordId}`, rec)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns-records-global', providerId] })
    },
  })
}

export function useAllDNSProviders() {
  return useQuery({
    queryKey: ['dns-providers', 'all'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<DNSProvider[]>>('/dns-providers')
      return data.data || []
    },
  })
}

