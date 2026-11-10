import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, BotCatalog, BotRulesResponse, UpdateBotRulesRequest } from '@/types'

// The API waits for the agent (up to 120s for a whole server), so the request
// must outlive the client's default 120s timeout.
const SAVE_TIMEOUT_MS = 150000

export function useBotCatalog() {
  return useQuery({
    queryKey: ['bot-catalog'],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<BotCatalog>>('/bot-catalog')
      return data.data!
    },
    staleTime: Infinity,
  })
}

export function useServerBotRules(serverId: number) {
  return useQuery({
    queryKey: ['bot-rules', 'server', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<BotRulesResponse>>(`/servers/${serverId}/bot-rules`)
      return data.data!
    },
    enabled: !!serverId,
  })
}

export function useUpdateServerBotRules(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: UpdateBotRulesRequest) => {
      const { data } = await api.put<APIResponse<BotRulesResponse>>(`/servers/${serverId}/bot-rules`, req, { timeout: SAVE_TIMEOUT_MS })
      return data
    },
    // Server defaults are shown as "inherited" on every site of the server.
    onSuccess: () => qc.invalidateQueries({ queryKey: ['bot-rules'] }),
  })
}

export function useSiteBotRules(siteId: number) {
  return useQuery({
    queryKey: ['bot-rules', 'site', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<BotRulesResponse>>(`/sites/${siteId}/bot-rules`)
      return data.data!
    },
    enabled: !!siteId,
  })
}

export function useUpdateSiteBotRules(siteId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: UpdateBotRulesRequest) => {
      const { data } = await api.put<APIResponse<BotRulesResponse>>(`/sites/${siteId}/bot-rules`, req, { timeout: SAVE_TIMEOUT_MS })
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['bot-rules', 'site', siteId] }),
  })
}

/** Extracts the API's error string from a failed request. */
export function botErrorMessage(err: unknown, fallback: string): string {
  const e = err as { response?: { data?: { error?: string } } }
  return e?.response?.data?.error || fallback
}
