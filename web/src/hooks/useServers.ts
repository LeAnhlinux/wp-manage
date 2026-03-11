import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { Server, CreateServerRequest, AutoDeployRequest, PaginatedResponse, APIResponse } from '@/types'

export function useServers(page = 1) {
  return useQuery({
    queryKey: ['servers', page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<Server>>(`/servers?page=${page}`)
      return data
    },
  })
}

export function useServer(id: number) {
  return useQuery({
    queryKey: ['server', id],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<Server>>(`/servers/${id}`)
      return data.data!
    },
    enabled: !!id,
  })
}

export function useCreateServer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateServerRequest) => {
      const { data } = await api.post('/servers', req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['servers'] }),
  })
}

export function useDeleteServer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/servers/${id}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['servers'] }),
  })
}

export function useAutoDeploy() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: AutoDeployRequest) => {
      const { data } = await api.post<APIResponse<{ ws_channel: string }>>('/servers/deploy', req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['servers'] }),
  })
}
