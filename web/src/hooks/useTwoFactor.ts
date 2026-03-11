import { useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { APIResponse, TwoFactorSetup } from '@/types'

export function useEnable2FA() {
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse<TwoFactorSetup>>('/auth/2fa/enable')
      return data.data!
    },
  })
}

export function useVerify2FA() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (code: string) => {
      const { data } = await api.post<APIResponse>('/auth/2fa/verify', { code })
      return data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['auth', 'me'] })
    },
  })
}

export function useDisable2FA() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse>('/auth/2fa/disable')
      return data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['auth', 'me'] })
    },
  })
}

export function useValidate2FA() {
  return useMutation({
    mutationFn: async (params: { email: string; password: string; code: string }) => {
      const { data } = await api.post<APIResponse<{ token: string; user: unknown }>>('/auth/2fa/validate', params)
      return data.data!
    },
  })
}
