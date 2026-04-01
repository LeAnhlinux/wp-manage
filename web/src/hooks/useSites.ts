import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type { Site, CreateSiteRequest, PaginatedResponse, APIResponse } from '@/types'

export function useSites(serverId?: number, page = 1) {
  return useQuery({
    queryKey: ['sites', serverId, page],
    queryFn: async () => {
      const params = new URLSearchParams({ page: String(page) })
      if (serverId) params.set('server_id', String(serverId))
      const { data } = await api.get<PaginatedResponse<Site>>(`/sites?${params}`)
      return data
    },
  })
}

export function useSite(id: number) {
  return useQuery({
    queryKey: ['site', id],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<Site>>(`/sites/${id}`)
      return data.data!
    },
    enabled: !!id,
  })
}

export function useCreateSite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateSiteRequest) => {
      const { data } = await api.post('/sites', req)
      return data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['sites'] }),
  })
}

export function useDeleteSite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/sites/${id}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['sites'] }),
  })
}

export function useUpdatePHP() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, version }: { id: number; version: string }) => {
      const { data } = await api.put(`/sites/${id}/php`, { version })
      return data
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ['site', v.id] })
      qc.invalidateQueries({ queryKey: ['sites'] })
    },
  })
}

export function useRestartSite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { data } = await api.post(`/sites/${id}/restart`)
      return data
    },
    onSuccess: (_d, id) => qc.invalidateQueries({ queryKey: ['site', id] }),
  })
}

export function useSuspendSite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { data } = await api.post(`/sites/${id}/suspend`)
      return data
    },
    onSuccess: (_d, id) => {
      qc.invalidateQueries({ queryKey: ['site', id] })
      qc.invalidateQueries({ queryKey: ['sites'] })
    },
  })
}

export function useResumeSite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { data } = await api.post(`/sites/${id}/resume`)
      return data
    },
    onSuccess: (_d, id) => {
      qc.invalidateQueries({ queryKey: ['site', id] })
      qc.invalidateQueries({ queryKey: ['sites'] })
    },
  })
}

export function useIssueSSL() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { data } = await api.post(`/sites/${id}/ssl`)
      return data
    },
    onSuccess: (_d, id) => {
      qc.invalidateQueries({ queryKey: ['site', id] })
      qc.invalidateQueries({ queryKey: ['sites'] })
    },
  })
}

export interface SiteCredentials {
  wp_admin_user: string
  wp_admin_pass: string
  wp_admin_email: string
  wp_admin_url: string
  db_name: string
  db_user: string
  db_password: string
}

export function useSiteCredentials(id: number, enabled: boolean) {
  return useQuery({
    queryKey: ['site-credentials', id],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<SiteCredentials>>(`/sites/${id}/credentials`)
      return data.data!
    },
    enabled: enabled && !!id,
  })
}

export function useAutologin() {
  return useMutation({
    mutationFn: async (id: number) => {
      const { data } = await api.post<APIResponse<{ url: string }>>(`/sites/${id}/autologin`)
      return data
    },
  })
}

export function useUploadSource() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({
      siteId, file, onProgress,
    }: { siteId: number; file: File; onProgress?: (pct: number) => void }) => {
      return new Promise<APIResponse<{ domain: string; file_size: number }>>((resolve, reject) => {
        const formData = new FormData()
        formData.append('file', file)
        const token = sessionStorage.getItem('token')
        const xhr = new XMLHttpRequest()
        xhr.open('POST', `/api/v1/sites/${siteId}/upload-source`)
        if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable) onProgress?.(Math.round((e.loaded / e.total) * 100))
        }
        xhr.onload = () => {
          try {
            const data = JSON.parse(xhr.responseText)
            if (xhr.status >= 200 && xhr.status < 300 && data.success) resolve(data)
            else reject(Object.assign(new Error(data.error || 'Upload failed'), { response: { data } }))
          } catch { reject(new Error('Invalid server response')) }
        }
        xhr.onerror = () => reject(new Error('Network error during upload'))
        xhr.send(formData)
      })
    },
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ['site', v.siteId] }),
  })
}

export function useImportDB() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({
      siteId, file, oldDomain, onProgress,
    }: { siteId: number; file: File; oldDomain?: string; onProgress?: (pct: number) => void }) => {
      return new Promise<APIResponse<{ domain: string; db_name: string; file_size: number }>>((resolve, reject) => {
        const formData = new FormData()
        formData.append('file', file)
        const token = sessionStorage.getItem('token')
        const url = oldDomain?.trim()
          ? `/api/v1/sites/${siteId}/import-db?old_domain=${encodeURIComponent(oldDomain.trim())}`
          : `/api/v1/sites/${siteId}/import-db`
        const xhr = new XMLHttpRequest()
        xhr.open('POST', url)
        if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable) onProgress?.(Math.round((e.loaded / e.total) * 100))
        }
        xhr.onload = () => {
          try {
            const data = JSON.parse(xhr.responseText)
            if (xhr.status >= 200 && xhr.status < 300 && data.success) resolve(data)
            else reject(Object.assign(new Error(data.error || 'Import failed'), { response: { data } }))
          } catch { reject(new Error('Invalid server response')) }
        }
        xhr.onerror = () => reject(new Error('Network error during upload'))
        xhr.send(formData)
      })
    },
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ['site', v.siteId] }),
  })
}

/** Toggle open_basedir on/off for a site. Rebuilds PHP-FPM pool synchronously. */
export function useToggleOpenBasedir() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (siteId: number) => {
      const { data } = await api.post<APIResponse<{ open_basedir_enabled: boolean }>>(
        `/sites/${siteId}/open-basedir/toggle`
      )
      return data
    },
    onSuccess: (_d, siteId) => qc.invalidateQueries({ queryKey: ['site', siteId] }),
  })
}
