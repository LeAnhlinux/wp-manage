import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, SSHKey, CreateSSHKeyRequest } from '@/types'

export function useSSHKeys(serverId: number) {
  return useQuery({
    queryKey: ['ssh-keys', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<SSHKey[]>>(`/servers/${serverId}/ssh-keys`)
      return data.data || []
    },
  })
}

export function useCreateSSHKey(serverId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateSSHKeyRequest) => {
      const { data } = await api.post<APIResponse<SSHKey>>(`/servers/${serverId}/ssh-keys`, req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ssh-keys', serverId] })
    },
  })
}

export function useDeleteSSHKey(serverId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (keyId: number) => {
      await api.delete(`/servers/${serverId}/ssh-keys/${keyId}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ssh-keys', serverId] })
    },
  })
}
