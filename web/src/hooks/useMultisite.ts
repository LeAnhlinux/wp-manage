import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, Subsite, EnableMultisiteRequest, CreateSubsiteRequest } from '@/types'

export function useSubsites(siteId: number, enabled: boolean) {
  return useQuery({
    queryKey: ['subsites', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<Subsite[]>>(`/sites/${siteId}/multisite/subsites`)
      return data.data ?? []
    },
    enabled,
  })
}

export function useEnableMultisite(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: EnableMultisiteRequest) => {
      const { data } = await api.post<APIResponse<null>>(`/sites/${siteId}/multisite/enable`, req)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['sites'] })
      qc.invalidateQueries({ queryKey: ['site', siteId] })
    },
  })
}

export function useCreateSubsite(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateSubsiteRequest) => {
      const { data } = await api.post<APIResponse<null>>(`/sites/${siteId}/multisite/subsites`, req)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['subsites', siteId] })
    },
  })
}

export function useDeleteSubsite(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (subsiteId: number) => {
      const { data } = await api.delete<APIResponse<null>>(`/sites/${siteId}/multisite/subsites/${subsiteId}`)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['subsites', siteId] })
    },
  })
}

export function useSyncSubsites(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse<null>>(`/sites/${siteId}/multisite/sync`)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['subsites', siteId] })
    },
  })
}
