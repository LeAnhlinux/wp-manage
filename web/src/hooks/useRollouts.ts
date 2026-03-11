import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, PaginatedResponse, AgentRelease, CreateReleaseRequest, RolloutPlan, CreateRolloutRequest } from '@/types'

// --- Releases ---

export function useReleases() {
  return useQuery({
    queryKey: ['releases'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<AgentRelease[]>>('/releases')
      return data
    },
  })
}

export function useCreateRelease() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateReleaseRequest) => {
      const { data } = await api.post<APIResponse<AgentRelease>>('/releases', req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['releases'] }),
  })
}

export function useDeleteRelease() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { data } = await api.delete<APIResponse>(`/releases/${id}`)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['releases'] }),
  })
}

// --- Rollouts ---

export function useRollouts(page = 1) {
  return useQuery({
    queryKey: ['rollouts', page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<RolloutPlan>>(`/rollouts?page=${page}&per_page=20`)
      return data
    },
  })
}

export function useRollout(id: number) {
  return useQuery({
    queryKey: ['rollout', id],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<RolloutPlan>>(`/rollouts/${id}`)
      return data
    },
    enabled: id > 0,
    refetchInterval: (query) => {
      const plan = query.state.data?.data
      return plan?.status === 'running' ? 5000 : false
    },
  })
}

export function useCreateRollout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateRolloutRequest) => {
      const { data } = await api.post<APIResponse<RolloutPlan>>('/rollouts', req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['rollouts'] }),
  })
}

export function useStartRollout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { data } = await api.post<APIResponse>(`/rollouts/${id}/start`)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['rollouts'] })
      qc.invalidateQueries({ queryKey: ['rollout'] })
    },
  })
}

export function usePauseRollout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { data } = await api.post<APIResponse>(`/rollouts/${id}/pause`)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['rollouts'] })
      qc.invalidateQueries({ queryKey: ['rollout'] })
    },
  })
}
