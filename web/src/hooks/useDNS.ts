import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, DNSProvider, CreateDNSProviderRequest, UpdateDNSRecordRequest, DNSRecord, CreateDNSRecordRequest } from '@/types'

export function useDNSProviders(serverId: number) {
  return useQuery({
    queryKey: ['dns-providers', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<DNSProvider[]>>(`/servers/${serverId}/dns-providers`)
      return data.data || []
    },
    enabled: !!serverId,
  })
}

export function useCreateDNSProvider(serverId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateDNSProviderRequest) => {
      const { data } = await api.post<APIResponse<DNSProvider>>(`/servers/${serverId}/dns-providers`, req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns-providers', serverId] })
    },
  })
}

export function useDeleteDNSProvider(serverId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (providerId: number) => {
      await api.delete(`/servers/${serverId}/dns-providers/${providerId}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns-providers', serverId] })
    },
  })
}

// Quick A-record upsert (legacy/convenience)
export function useUpdateDNSRecord(serverId: number) {
  return useMutation({
    mutationFn: async ({ providerId, ...req }: UpdateDNSRecordRequest & { providerId: number }) => {
      const { data } = await api.post<APIResponse>(`/servers/${serverId}/dns-providers/${providerId}/update-record`, req)
      return data
    },
  })
}

// Full DNS records CRUD for a provider's zone
export function useDNSRecords(serverId: number, providerId: number | null) {
  return useQuery({
    queryKey: ['dns-records', serverId, providerId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<DNSRecord[]>>(
        `/servers/${serverId}/dns-providers/${providerId}/records`
      )
      return data.data || []
    },
    enabled: !!serverId && !!providerId,
  })
}

export function useCreateDNSRecord(serverId: number, providerId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateDNSRecordRequest) => {
      const { data } = await api.post<APIResponse<DNSRecord>>(
        `/servers/${serverId}/dns-providers/${providerId}/records`,
        req
      )
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns-records', serverId, providerId] })
    },
  })
}

export function useDeleteDNSRecord(serverId: number, providerId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (recordId: string) => {
      await api.delete(`/servers/${serverId}/dns-providers/${providerId}/records/${recordId}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns-records', serverId, providerId] })
    },
  })
}
