import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type {
  FileInfo,
  FileContentRequest,
  FileMkdirRequest,
  FileRenameRequest,
  FileDeleteRequest,
  FilePermissionsRequest,
  FileSearchRequest,
  FileSearchResult,
  FileCopyMoveRequest,
  FileDeleteBulkRequest,
  APIResponse,
} from '@/types'

export function useFileList(siteId: number, path: string) {
  return useQuery({
    queryKey: ['files', siteId, path],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<FileInfo[]>>(
        `/sites/${siteId}/files?path=${encodeURIComponent(path)}`
      )
      return data.data ?? []
    },
    enabled: !!siteId,
  })
}

export function useFileContent(siteId: number, path: string, enabled: boolean) {
  return useQuery({
    queryKey: ['file-content', siteId, path],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<{ content: string; path: string }>>(
        `/sites/${siteId}/files/content?path=${encodeURIComponent(path)}`
      )
      return data.data!
    },
    enabled: enabled && !!siteId && !!path,
  })
}

export function useSaveFileContent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, req }: { siteId: number; req: FileContentRequest }) => {
      const { data } = await api.put<APIResponse>(`/sites/${siteId}/files/content`, req)
      return data
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ['file-content', v.siteId, v.req.path] })
      // Invalidate file list to update mod_time/size
      qc.invalidateQueries({ queryKey: ['files', v.siteId] })
    },
  })
}

export function useFileMkdir() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, req }: { siteId: number; req: FileMkdirRequest }) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/files/mkdir`, req)
      return data
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ['files', v.siteId] })
    },
  })
}

export function useFileRename() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, req }: { siteId: number; req: FileRenameRequest }) => {
      const { data } = await api.put<APIResponse>(`/sites/${siteId}/files/rename`, req)
      return data
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ['files', v.siteId] })
    },
  })
}

export function useFileDelete() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, req }: { siteId: number; req: FileDeleteRequest }) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/files/delete`, req)
      return data
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ['files', v.siteId] })
    },
  })
}

export function useFilePermissions() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, req }: { siteId: number; req: FilePermissionsRequest }) => {
      const { data } = await api.put<APIResponse>(`/sites/${siteId}/files/permissions`, req)
      return data
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ['files', v.siteId] })
    },
  })
}

export function useFileUpload() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, path, files }: { siteId: number; path: string; files: File[] }) => {
      const formData = new FormData()
      for (const file of files) {
        formData.append('files', file)
      }
      const { data } = await api.post<APIResponse>(
        `/sites/${siteId}/files/upload?path=${encodeURIComponent(path)}`,
        formData,
        { headers: { 'Content-Type': 'multipart/form-data' } }
      )
      return data
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ['files', v.siteId] })
    },
  })
}

const CHUNK_SIZE = 10 * 1024 * 1024 // 10MB per chunk

export function useFileUploadChunked() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({
      siteId, path, file, onProgress,
    }: {
      siteId: number
      path: string
      file: File
      onProgress?: (pct: number) => void
    }) => {
      const token = localStorage.getItem('token')
      const authHeader = token ? `Bearer ${token}` : ''

      if (file.size <= CHUNK_SIZE) {
        // Small file: single multipart upload with XHR for real progress tracking
        await new Promise<void>((resolve, reject) => {
          const xhr = new XMLHttpRequest()
          const formData = new FormData()
          formData.append('files', file)
          xhr.upload.onprogress = (e) => {
            if (e.lengthComputable) onProgress?.(Math.round((e.loaded / e.total) * 100))
          }
          xhr.onload = () => {
            if (xhr.status >= 200 && xhr.status < 300) {
              onProgress?.(100)
              resolve()
            } else {
              reject(new Error('Upload failed'))
            }
          }
          xhr.onerror = () => reject(new Error('Network error'))
          xhr.open('POST', `/api/v1/sites/${siteId}/files/upload?path=${encodeURIComponent(path)}`)
          xhr.setRequestHeader('Authorization', authHeader)
          xhr.send(formData)
        })
        return
      }

      // Large file: chunked upload
      const totalChunks = Math.ceil(file.size / CHUNK_SIZE)
      const uploadId = `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`

      for (let i = 0; i < totalChunks; i++) {
        const chunk = file.slice(i * CHUNK_SIZE, (i + 1) * CHUNK_SIZE)
        const params = new URLSearchParams({
          upload_id: uploadId,
          chunk_index: String(i),
          filename: file.name,
        })
        const resp = await fetch(`/api/v1/sites/${siteId}/files/upload-chunk?${params}`, {
          method: 'POST',
          headers: { Authorization: authHeader, 'Content-Type': 'application/octet-stream' },
          body: chunk,
        })
        if (!resp.ok) throw new Error(`Chunk ${i + 1}/${totalChunks} upload failed`)
        onProgress?.(Math.round(((i + 1) / totalChunks) * 90)) // reserve last 10% for merge
      }

      // Merge all chunks
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/files/upload-merge`, {
        upload_id: uploadId,
        filename: file.name,
        total_chunks: totalChunks,
        dest_path: path,
      })
      if (!data.success) throw new Error(data.error || 'Merge failed')
      onProgress?.(100)
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ['files', v.siteId] })
    },
  })
}

export function useFileExtract() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, path, destPath }: { siteId: number; path: string; destPath?: string }) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/files/extract`, {
        path,
        dest_path: destPath ?? '',
      })
      return data
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ['files', v.siteId] })
    },
  })
}

export function useFileSearch() {
  return useMutation({
    mutationFn: async ({ siteId, req }: { siteId: number; req: FileSearchRequest }) => {
      const { data } = await api.post<APIResponse<FileSearchResult[]>>(
        `/sites/${siteId}/files/search`,
        req
      )
      return data.data ?? []
    },
  })
}

export function useFileCopy() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, req }: { siteId: number; req: FileCopyMoveRequest }) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/files/copy`, req)
      return data
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ['files', v.siteId] })
    },
  })
}

export function useFileMove() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, req }: { siteId: number; req: FileCopyMoveRequest }) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/files/move`, req)
      return data
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ['files', v.siteId] })
    },
  })
}

export function useFileDeleteBulk() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ siteId, req }: { siteId: number; req: FileDeleteBulkRequest }) => {
      const { data } = await api.post<APIResponse>(`/sites/${siteId}/files/delete-bulk`, req)
      return data
    },
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ['files', v.siteId] })
    },
  })
}

export function getFileDownloadUrl(siteId: number, path: string): string {
  const token = localStorage.getItem('token')
  return `/api/v1/sites/${siteId}/files/download?path=${encodeURIComponent(path)}&token=${token}`
}
