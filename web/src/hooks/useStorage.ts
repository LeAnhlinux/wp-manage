import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, StorageCredential, CreateStorageCredentialRequest } from '@/types'

export function useStorageCredentials(serverId: number) {
  return useQuery({
    queryKey: ['storage-creds', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<StorageCredential[]>>(`/servers/${serverId}/storage`)
      return data.data || []
    },
    enabled: !!serverId,
  })
}

export function useCreateStorageCredential(serverId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateStorageCredentialRequest) => {
      const { data } = await api.post<APIResponse<StorageCredential>>(`/servers/${serverId}/storage`, req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['storage-creds', serverId] })
    },
  })
}

export function useDeleteStorageCredential(serverId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (credId: number) => {
      await api.delete(`/servers/${serverId}/storage/${credId}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['storage-creds', serverId] })
    },
  })
}
