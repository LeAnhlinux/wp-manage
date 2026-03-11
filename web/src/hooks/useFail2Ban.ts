import { useQuery, useMutation } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, Fail2BanUnbanRequest, Fail2BanUpdateJailRequest } from '@/types'

export function useFail2BanStatus(serverId: number) {
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.get<APIResponse>(`/servers/${serverId}/fail2ban`)
      return data
    },
  })
}

export function useFail2BanUnban(serverId: number) {
  return useMutation({
    mutationFn: async (req: Fail2BanUnbanRequest) => {
      const { data } = await api.post<APIResponse>(`/servers/${serverId}/fail2ban/unban`, req)
      return data
    },
  })
}

export function useFail2BanUpdateJail(serverId: number) {
  return useMutation({
    mutationFn: async (req: Fail2BanUpdateJailRequest) => {
      const { data } = await api.put<APIResponse>(`/servers/${serverId}/fail2ban/jail`, req)
      return data
    },
  })
}
