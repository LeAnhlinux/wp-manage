import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, GeoCatalog, GeoRulesResponse, UpdateGeoRulesRequest } from '@/types'

// The API waits for the agent (up to 180s for a whole server; the first apply
// installs an nginx module), so saves must outlive the default 120s timeout.
const SAVE_TIMEOUT_MS = 210000

export function useGeoCatalog() {
  return useQuery({
    queryKey: ['geo-catalog'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<GeoCatalog>>('/geo/catalog')
      return data.data!
    },
    staleTime: 5 * 60 * 1000,
  })
}

export function useRefreshGeoProviders() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse<GeoCatalog>>('/geo/providers/refresh', {}, { timeout: SAVE_TIMEOUT_MS })
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['geo-catalog'] }),
  })
}

export function useServerGeoRules(serverId: number) {
  return useQuery({
    queryKey: ['geo-rules', 'server', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<GeoRulesResponse>>(`/servers/${serverId}/geo-rules`)
      return data.data!
    },
    enabled: !!serverId,
  })
}

export function useUpdateServerGeoRules(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: UpdateGeoRulesRequest) => {
      const { data } = await api.put<APIResponse<GeoRulesResponse>>(`/servers/${serverId}/geo-rules`, req, { timeout: SAVE_TIMEOUT_MS })
      return data
    },
    // Server defaults are shown on every site of the server.
    onSuccess: () => qc.invalidateQueries({ queryKey: ['geo-rules'] }),
  })
}

export function useSiteGeoRules(siteId: number) {
  return useQuery({
    queryKey: ['geo-rules', 'site', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<GeoRulesResponse>>(`/sites/${siteId}/geo-rules`)
      return data.data!
    },
    enabled: !!siteId,
  })
}

export function useUpdateSiteGeoRules(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: UpdateGeoRulesRequest) => {
      const { data } = await api.put<APIResponse<GeoRulesResponse>>(`/sites/${siteId}/geo-rules`, req, { timeout: SAVE_TIMEOUT_MS })
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['geo-rules', 'site', siteId] }),
  })
}

/** Extracts the API's error string from a failed request. */
export function geoErrorMessage(err: unknown, fallback: string): string {
  const e = err as { response?: { data?: { error?: string } } }
  return e?.response?.data?.error || fallback
}
