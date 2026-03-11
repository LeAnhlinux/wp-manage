import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIKey, CreateAPIKeyRequest, APIResponse } from '@/types'

export function useAPIKeys() {
  return useQuery({
    queryKey: ['api-keys'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<APIKey[]>>('/api-keys')
      return data.data || []
    },
  })
}

export function useCreateAPIKey() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateAPIKeyRequest) => {
      const { data } = await api.post<APIResponse<{ api_key: APIKey; key: string }>>('/api-keys', req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['api-keys'] }),
  })
}

export function useDeleteAPIKey() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/api-keys/${id}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['api-keys'] }),
  })
}
