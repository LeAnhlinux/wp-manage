import { useMutation } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, HardenWordPressRequest } from '@/types'

export function useHardenWordPress(siteId: number) {
  return useMutation({
    mutationFn: async (req: HardenWordPressRequest) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/harden`, req)
      return data
    },
  })
}
