import { useQuery, useMutation } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, SSLStatus, UploadCustomSSLRequest } from '@/types'

export function useSSLStatus(siteId: number) {
  return useQuery({
    queryKey: ['ssl-status', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<SSLStatus>>(`/sites/${siteId}/ssl/status`)
      return data.data!
    },
    enabled: !!siteId,
  })
}

export function useEnableSSL(siteId: number) {
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/ssl/enable`)
      return data
    },
  })
}

export function useDisableSSL(siteId: number) {
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/ssl/disable`)
      return data
    },
  })
}

export function useRenewSSL(siteId: number) {
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/ssl/renew`)
      return data
    },
  })
}

export function useUploadCustomSSL(siteId: number) {
  return useMutation({
    mutationFn: async (req: UploadCustomSSLRequest) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/ssl/custom`, req)
      return data
    },
  })
}

export function useForceHTTPS(siteId: number) {
  return useMutation({
    mutationFn: async (enable: boolean) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/ssl/force-https`, { enable })
      return data
    },
  })
}
