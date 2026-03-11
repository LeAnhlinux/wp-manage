import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, SFTPAccount, CreateSFTPRequest } from '@/types'

export function useSFTPAccounts(siteId: number) {
  return useQuery({
    queryKey: ['sftp', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<SFTPAccount[]>>(`/sites/${siteId}/sftp`)
      return data.data || []
    },
  })
}

export function useCreateSFTP(siteId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateSFTPRequest) => {
      const { data } = await api.post<APIResponse<SFTPAccount>>(`/sites/${siteId}/sftp`, req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sftp', siteId] })
    },
  })
}

export function useDeleteSFTP(siteId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (accountId: number) => {
      await api.delete(`/sites/${siteId}/sftp/${accountId}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sftp', siteId] })
    },
  })
}
