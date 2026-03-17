import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { IonCubeVersionStatus } from '@/types'

interface APIResponse<T = unknown> {
  success: boolean
  data?: T
  message?: string
  error?: string
}

export function useIonCubeStatus(serverId: number) {
  return useQuery({
    queryKey: ['ioncube-status', serverId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<IonCubeVersionStatus[]>>(
        `/servers/${serverId}/ioncube`
      )
      return data.data ?? []
    },
    enabled: !!serverId,
  })
}

export function useEnableIonCube(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (phpVersion: string) => {
      const { data } = await api.post<APIResponse>(
        `/servers/${serverId}/ioncube/${phpVersion}/enable`
      )
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['ioncube-status', serverId] })
    },
  })
}

export function useDisableIonCube(serverId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (phpVersion: string) => {
      const { data } = await api.post<APIResponse>(
        `/servers/${serverId}/ioncube/${phpVersion}/disable`
      )
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['ioncube-status', serverId] })
    },
  })
}
