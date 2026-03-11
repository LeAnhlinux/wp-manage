import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { RedisConfigData, RedisConfigRequest, APIResponse, RedisSiteStatus } from '@/types'

export function useRedisConfig(siteId: number) {
  return useQuery({
    queryKey: ['redis-config', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<RedisConfigData>>(`/sites/${siteId}/redis`)
      return data.data!
    },
    enabled: !!siteId,
  })
}

export function useUpdateRedisConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, ...req }: RedisConfigRequest & { siteId: number }) => {
      const { data } = await api.put<APIResponse<RedisConfigData>>(`/sites/${siteId}/redis`, req)
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['redis-config', vars.siteId] })
    },
  })
}

export function useToggleRedis() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, enabled }: { siteId: number; enabled: boolean }) => {
      const { data } = await api.post<APIResponse<RedisConfigData>>(`/sites/${siteId}/redis/toggle`, { enabled })
      return data
    },
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ['redis-config', vars.siteId] })
    },
  })
}

export function useRedisStats(siteId: number) {
  return useQuery({
    queryKey: ['redis-stats', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<{ raw: string }>>(`/sites/${siteId}/redis/stats`)
      return data.data?.raw ?? ''
    },
    enabled: false, // only fetch on demand
    staleTime: 0,
  })
}

export function useFlushRedis() {
  return useMutation({
    mutationFn: async ({ siteId }: { siteId: number }) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/redis/flush`)
      return data
    },
  })
}

// ─── Server-level Redis hooks ─────────────────────────────────────────────────

export function useServerRedisStats(serverId: number) {
  return useQuery({
    queryKey: ['server-redis-stats', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<{ raw: string }>>(`/servers/${serverId}/redis/stats`)
      return data.data?.raw ?? ''
    },
    enabled: false, // on demand
    staleTime: 0,
  })
}

export function useFlushServerRedis() {
  return useMutation({
    mutationFn: async ({ serverId }: { serverId: number }) => {
      const { data } = await api.post<APIResponse>(`/servers/${serverId}/redis/flush`)
      return data
    },
  })
}

export function useServerRedisSiteStatuses(serverId: number) {
  const qc = useQueryClient()
  return {
    query: useQuery({
      queryKey: ['server-redis-sites', serverId],
      queryFn: async () => {
        const { data } = await api.get<APIResponse<RedisSiteStatus[]>>(`/servers/${serverId}/redis/sites`)
        return data.data ?? []
      },
      enabled: !!serverId,
    }),
    invalidate: () => qc.invalidateQueries({ queryKey: ['server-redis-sites', serverId] }),
  }
}
