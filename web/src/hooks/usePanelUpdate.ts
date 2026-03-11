import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, PanelUpdateStatus, PanelRelease, CreatePanelReleaseRequest } from '@/types'

export function usePanelUpdateStatus() {
  return useQuery({
    queryKey: ['panel-update'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<PanelUpdateStatus>>('/settings/panel-update')
      return data.data!
    },
  })
}

export function usePanelReleases() {
  return useQuery({
    queryKey: ['panel-releases'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<PanelRelease[]>>('/settings/panel-update/releases')
      return data.data || []
    },
  })
}

export function useCreatePanelRelease() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreatePanelReleaseRequest) => {
      const { data } = await api.post<APIResponse<PanelRelease>>('/settings/panel-update/releases', req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['panel-releases'] })
      queryClient.invalidateQueries({ queryKey: ['panel-update'] })
    },
  })
}
