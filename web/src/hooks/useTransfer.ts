import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type {
  TransferJob,
  ConflictCheckResult,
  TransferConnectRequest,
  TransferStartRequest,
  TransferTestConnectionRequest,
  TransferTestConnectionResult,
  PaginatedResponse,
  APIResponse,
} from '@/types'

export function useTestConnection() {
  return useMutation({
    mutationFn: async (req: TransferTestConnectionRequest) => {
      const { data } = await api.post<APIResponse<TransferTestConnectionResult>>(
        '/transfer/test-connection',
        req
      )
      return data.data!
    },
  })
}

export function useTransferJobs(page = 1) {
  return useQuery({
    queryKey: ['transfer', 'list', page],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<TransferJob>>(`/transfer?page=${page}`)
      return data
    },
    refetchInterval: (query) => {
      const hasActive = query.state.data?.data?.some(
        (j) => j.status === 'discovering' || j.status === 'transferring'
      )
      return hasActive ? 3000 : false
    },
  })
}

export function useTransferJob(id: number | null) {
  return useQuery({
    queryKey: ['transfer', id],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<TransferJob>>(`/transfer/${id}`)
      return data.data!
    },
    enabled: !!id,
    refetchInterval: (query) => {
      const job = query.state.data
      if (job && (job.status === 'discovering' || job.status === 'transferring')) return 2000
      return false
    },
  })
}

export function useConnectTransfer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: TransferConnectRequest) => {
      const { data } = await api.post<APIResponse<TransferJob>>('/transfer/connect', req)
      return data.data!
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['transfer', 'list'] })
    },
  })
}

export function useStartTransfer(id: number | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: TransferStartRequest) => {
      const { data } = await api.post<APIResponse<TransferJob>>(`/transfer/${id}/start`, req)
      return data.data!
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['transfer', id] })
      qc.invalidateQueries({ queryKey: ['transfer', 'list'] })
    },
  })
}

export function useCancelTransfer(id: number | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse>(`/transfer/${id}/cancel`)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['transfer', id] })
      qc.invalidateQueries({ queryKey: ['transfer', 'list'] })
    },
  })
}

export function useDeleteTransferJob() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { data } = await api.delete<APIResponse>(`/transfer/${id}`)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['transfer', 'list'] })
    },
  })
}

export function useCheckDomain(
  domain: string,
  targetServerId: number | null,
  jobId: number | null,
  enabled = false
) {
  return useQuery({
    queryKey: ['transfer', 'check-domain', domain, targetServerId, jobId],
    queryFn: async () => {
      const params = new URLSearchParams({
        domain,
        target_server_id: String(targetServerId),
        ...(jobId ? { job_id: String(jobId) } : {}),
      })
      const { data } = await api.get<APIResponse<ConflictCheckResult>>(
        `/transfer/check-domain?${params.toString()}`
      )
      return data.data!
    },
    enabled: !!domain && !!targetServerId && enabled,
    staleTime: 10000,
    retry: false,
  })
}
