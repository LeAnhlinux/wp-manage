import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { User, CreateUserRequest, UpdateUserRequest, AssignServerRequest, Server, PaginatedResponse, APIResponse } from '@/types'

export function useUsers(page = 1) {
  return useQuery({
    queryKey: ['users', page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<User>>(`/users?page=${page}`)
      return data
    },
  })
}

export function useUser(id: number) {
  return useQuery({
    queryKey: ['user', id],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<User>>(`/users/${id}`)
      return data.data!
    },
    enabled: !!id,
  })
}

export function useCreateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateUserRequest) => {
      const { data } = await api.post('/users', req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useUpdateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, ...req }: UpdateUserRequest & { id: number }) => {
      const { data } = await api.put(`/users/${id}`, req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useDeleteUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/users/${id}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useAssignServer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ userId, ...req }: AssignServerRequest & { userId: number }) => {
      const { data } = await api.post(`/users/${userId}/servers`, req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['user-servers'] }),
  })
}

export function useUnassignServer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ userId, serverId }: { userId: number; serverId: number }) => {
      await api.delete(`/users/${userId}/servers/${serverId}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['user-servers'] }),
  })
}

export function useUserServers(userId: number) {
  return useQuery({
    queryKey: ['user-servers', userId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<Server[]>>(`/users/${userId}/servers`)
      return data.data || []
    },
    enabled: !!userId,
  })
}
