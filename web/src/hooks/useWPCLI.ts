import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, WPCLIHistory, WPCLIRequest } from '@/types'

export function useWPCLIHistory(siteId: number) {
  return useQuery({
    queryKey: ['wpcli-history', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<WPCLIHistory[]>>(`/sites/${siteId}/wpcli/history`)
      return data.data || []
    },
    enabled: !!siteId,
  })
}

export function useExecuteWPCLI(siteId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: WPCLIRequest) => {
      const { data } = await api.post<APIResponse<WPCLIHistory>>(`/sites/${siteId}/wpcli`, req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['wpcli-history', siteId] })
    },
  })
}
