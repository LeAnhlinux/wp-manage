import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, Fail2BanUnbanRequest, Fail2BanUpdateJailRequest } from '@/types'

export function useFail2BanStatus(serverId: number) {
  return useQuery({
    queryKey: ['fail2ban', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse>(`/servers/${serverId}/fail2ban`)
      return data.data
    },
    enabled: !!serverId,
    staleTime: 0,
    refetchOnWindowFocus: false,
  })
}

export function useFail2BanUnban(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: Fail2BanUnbanRequest) => {
      const { data } = await api.post<APIResponse>(`/servers/${serverId}/fail2ban/unban`, req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['fail2ban', serverId] }),
  })
}

export function useFail2BanUpdateJail(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: Fail2BanUpdateJailRequest) => {
      const { data } = await api.put<APIResponse>(`/servers/${serverId}/fail2ban/jail`, req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['fail2ban', serverId] }),
  })
}

export function useFail2BanToggleService(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (enable: boolean) => {
      const { data } = await api.post<APIResponse>(`/servers/${serverId}/fail2ban/service`, { enable })
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['fail2ban', serverId] }),
  })
}

export function useFail2BanSetupJails(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse>(`/servers/${serverId}/fail2ban/setup`)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['fail2ban', serverId] }),
  })
}
