import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, ExternalImport, ImportExternalRequest } from '@/types'

export function useImports(siteId: number) {
  return useQuery({
    queryKey: ['imports', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<ExternalImport[]>>(`/sites/${siteId}/import`)
      return data.data || []
    },
    enabled: !!siteId,
  })
}

export function useImportStatus(siteId: number, importId: number) {
  return useQuery({
    queryKey: ['import-status', siteId, importId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<ExternalImport>>(`/sites/${siteId}/import/${importId}`)
      return data.data!
    },
    enabled: !!siteId && !!importId,
    refetchInterval: 5000,
  })
}

export function useStartImport(siteId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: ImportExternalRequest) => {
      const { data } = await api.post<APIResponse<ExternalImport>>(`/sites/${siteId}/import`, req)
      return data.data!
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['imports', siteId] })
    },
  })
}
