import { useState, useCallback, useRef } from 'react'
import {
  Folder, File, ChevronRight, Home, Upload, FolderPlus,
  Download, Trash2, Edit3, Lock, MoreVertical, Search,
  ArrowLeft, RefreshCw, X, Save, FileText, Image, Code,
  Archive, Film, Music, FileSpreadsheet, PackageOpen,
  Copy, Scissors, Clipboard, CheckSquare, Square
} from 'lucide-react'
import {
  useFileList, useFileMkdir, useFileRename, useFileDelete,
  useFilePermissions, useFileUploadChunked, useFileExtract,
  useFileSearch, useFileCopy, useFileMove, useFileDeleteBulk,
  getFileDownloadUrl
} from '@/hooks/useFiles'
import { formatBytes, formatDate } from '@/lib/utils'
import type { FileInfo, FileSearchResult } from '@/types'
import FileEditor from './FileEditor'

interface Props {
  siteId: number
}

const TEXT_EXTENSIONS = new Set([
  'php', 'js', 'ts', 'jsx', 'tsx', 'css', 'scss', 'less', 'html', 'htm',
  'xml', 'json', 'yml', 'yaml', 'md', 'txt', 'conf', 'cfg', 'ini', 'env',
  'htaccess', 'log', 'sh', 'bash', 'sql', 'py', 'rb', 'tpl', 'twig', 'vue', 'svg',
])

const ARCHIVE_EXTENSIONS = new Set(['zip', 'tar', 'gz', 'bz2', 'tgz'])

function getFileExtension(name: string): string {
  const idx = name.lastIndexOf('.')
  return idx > 0 ? name.substring(idx + 1).toLowerCase() : ''
}

function isTextFile(name: string): boolean {
  return TEXT_EXTENSIONS.has(getFileExtension(name))
}

function isArchiveFile(name: string): boolean {
  return ARCHIVE_EXTENSIONS.has(getFileExtension(name))
}

function getFileIcon(file: FileInfo) {
  if (file.is_dir) return <Folder size={18} className="text-yellow-500" />
  const ext = getFileExtension(file.name)
  if (['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'ico', 'bmp'].includes(ext))
    return <Image size={18} className="text-purple-500" />
  if (['php', 'js', 'ts', 'jsx', 'tsx', 'py', 'rb', 'sh'].includes(ext))
    return <Code size={18} className="text-green-500" />
  if (['html', 'htm', 'css', 'scss', 'less'].includes(ext))
    return <Code size={18} className="text-orange-500" />
  if (['zip', 'tar', 'gz', 'rar', '7z', 'bz2'].includes(ext))
    return <Archive size={18} className="text-red-500" />
  if (['mp4', 'avi', 'mov', 'webm', 'mkv'].includes(ext))
    return <Film size={18} className="text-blue-500" />
  if (['mp3', 'wav', 'ogg', 'flac'].includes(ext))
    return <Music size={18} className="text-pink-500" />
  if (['csv', 'xls', 'xlsx'].includes(ext))
    return <FileSpreadsheet size={18} className="text-green-600" />
  if (['txt', 'md', 'log'].includes(ext))
    return <FileText size={18} className="text-gray-500" />
  return <File size={18} className="text-gray-400" />
}

// Clipboard state
interface ClipboardState {
  paths: string[]
  op: 'copy' | 'cut'
}

export default function FileBrowser({ siteId }: Props) {
  const [currentPath, setCurrentPath] = useState('/')
  const [editingFile, setEditingFile] = useState<string | null>(null)
  const [contextMenu, setContextMenu] = useState<{ file: FileInfo; x: number; y: number } | null>(null)
  const [showNewFolder, setShowNewFolder] = useState(false)
  const [newFolderName, setNewFolderName] = useState('')
  const [renaming, setRenaming] = useState<FileInfo | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [showChmod, setShowChmod] = useState<FileInfo | null>(null)
  const [chmodValue, setChmodValue] = useState('')
  const [chmodRecursive, setChmodRecursive] = useState(false)
  const [showSearch, setShowSearch] = useState(false)
  const [searchPattern, setSearchPattern] = useState('')
  const [searchResults, setSearchResults] = useState<FileSearchResult[]>([])
  const [isDragging, setIsDragging] = useState(false)
  const [actionMsg, setActionMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [uploadProgress, setUploadProgress] = useState<{ filename: string; pct: number } | null>(null)
  const [extractPrompt, setExtractPrompt] = useState<{ path: string; name: string } | null>(null)

  // Multi-select state
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [clipboard, setClipboard] = useState<ClipboardState | null>(null)

  const fileInputRef = useRef<HTMLInputElement>(null)

  const { data: files, isLoading, refetch } = useFileList(siteId, currentPath)
  const mkdirMut = useFileMkdir()
  const renameMut = useFileRename()
  const deleteMut = useFileDelete()
  const chmodMut = useFilePermissions()
  const uploadMut = useFileUploadChunked()
  const extractMut = useFileExtract()
  const searchMut = useFileSearch()
  const copyMut = useFileCopy()
  const moveMut = useFileMove()
  const deleteBulkMut = useFileDeleteBulk()

  const showMsg = (type: 'success' | 'error', text: string) => {
    setActionMsg({ type, text })
    setTimeout(() => setActionMsg(null), 4000)
  }

  // Navigation
  const navigateTo = (path: string) => {
    setCurrentPath(path)
    setContextMenu(null)
    setSelected(new Set()) // clear selection on navigate
  }

  const goUp = () => {
    if (currentPath === '/') return
    const parent = currentPath.replace(/\/[^/]+\/?$/, '') || '/'
    navigateTo(parent)
  }

  const pathParts = currentPath.split('/').filter(Boolean)

  // File click handler - skip navigation if in selection mode
  const handleFileClick = (file: FileInfo, e: React.MouseEvent) => {
    // Ctrl/Cmd+click = toggle selection
    if (e.ctrlKey || e.metaKey) {
      e.preventDefault()
      toggleSelect(file.path)
      return
    }
    // If there are selections, clicking acts as toggle
    if (selected.size > 0) {
      toggleSelect(file.path)
      return
    }
    // Normal click
    if (file.is_dir) {
      navigateTo(file.path)
    } else if (isTextFile(file.name)) {
      setEditingFile(file.path)
    }
  }

  // Checkbox click
  const handleCheckboxClick = (e: React.MouseEvent, path: string) => {
    e.stopPropagation()
    toggleSelect(path)
  }

  const toggleSelect = (path: string) => {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }

  const selectAll = () => {
    if (!files) return
    setSelected(new Set(files.map(f => f.path)))
  }

  const clearSelection = () => setSelected(new Set())

  // Context menu
  const handleContextMenu = (e: React.MouseEvent, file: FileInfo) => {
    e.preventDefault()
    e.stopPropagation()
    // If right-clicking an unselected item, select only it
    if (!selected.has(file.path)) {
      setSelected(new Set([file.path]))
    }
    setContextMenu({ file, x: e.clientX, y: e.clientY })
  }

  // Close context menu on click outside
  const closeContextMenu = useCallback(() => setContextMenu(null), [])

  // New folder
  const handleCreateFolder = async () => {
    if (!newFolderName.trim()) return
    try {
      const fullPath = currentPath === '/' ? `/${newFolderName}` : `${currentPath}/${newFolderName}`
      await mkdirMut.mutateAsync({ siteId, req: { path: fullPath } })
      setShowNewFolder(false)
      setNewFolderName('')
      showMsg('success', `Created folder "${newFolderName}"`)
    } catch {
      showMsg('error', 'Failed to create folder')
    }
  }

  // Rename
  const handleRename = async () => {
    if (!renaming || !renameValue.trim()) return
    try {
      const dir = renaming.path.replace(/\/[^/]+$/, '')
      const newPath = `${dir}/${renameValue}`
      await renameMut.mutateAsync({ siteId, req: { old_path: renaming.path, new_path: newPath } })
      setRenaming(null)
      setRenameValue('')
      showMsg('success', `Renamed to "${renameValue}"`)
    } catch {
      showMsg('error', 'Failed to rename')
    }
  }

  // Delete single
  const handleDelete = async (file: FileInfo) => {
    if (!confirm(`Delete ${file.is_dir ? 'folder' : 'file'} "${file.name}"?`)) return
    setContextMenu(null)
    try {
      await deleteMut.mutateAsync({ siteId, req: { path: file.path } })
      showMsg('success', `Deleted "${file.name}"`)
    } catch {
      showMsg('error', 'Failed to delete')
    }
  }

  // Delete bulk (selected)
  const handleDeleteSelected = async () => {
    if (selected.size === 0) return
    if (!confirm(`Delete ${selected.size} item(s)?`)) return
    setContextMenu(null)
    try {
      await deleteBulkMut.mutateAsync({ siteId, req: { paths: Array.from(selected) } })
      showMsg('success', `Deleted ${selected.size} item(s)`)
      clearSelection()
    } catch {
      showMsg('error', 'Failed to delete selected items')
    }
  }

  // Copy selected to clipboard
  const handleCopySelected = () => {
    if (selected.size === 0) return
    setClipboard({ paths: Array.from(selected), op: 'copy' })
    setContextMenu(null)
    showMsg('success', `${selected.size} item(s) copied to clipboard`)
  }

  // Cut selected to clipboard
  const handleCutSelected = () => {
    if (selected.size === 0) return
    setClipboard({ paths: Array.from(selected), op: 'cut' })
    setContextMenu(null)
    showMsg('success', `${selected.size} item(s) cut to clipboard`)
  }

  // Paste clipboard into currentPath
  const handlePaste = async () => {
    if (!clipboard) return
    setContextMenu(null)
    try {
      if (clipboard.op === 'copy') {
        await copyMut.mutateAsync({ siteId, req: { src_paths: clipboard.paths, dest_dir: currentPath } })
        showMsg('success', `Pasted ${clipboard.paths.length} item(s)`)
      } else {
        await moveMut.mutateAsync({ siteId, req: { src_paths: clipboard.paths, dest_dir: currentPath } })
        showMsg('success', `Moved ${clipboard.paths.length} item(s)`)
        setClipboard(null)
      }
    } catch {
      showMsg('error', 'Paste failed')
    }
  }

  // Chmod
  const handleChmod = async () => {
    if (!showChmod || !chmodValue.trim()) return
    try {
      await chmodMut.mutateAsync({
        siteId,
        req: { path: showChmod.path, permissions: chmodValue, recursive: chmodRecursive },
      })
      setShowChmod(null)
      setChmodValue('')
      setChmodRecursive(false)
      showMsg('success', 'Permissions updated')
    } catch {
      showMsg('error', 'Failed to update permissions')
    }
  }

  // Upload
  const handleUpload = async (fileList: FileList | null) => {
    if (!fileList || fileList.length === 0) return
    const fileArr = Array.from(fileList)
    let lastZipPath: { path: string; name: string } | null = null
    let failed = 0
    for (const file of fileArr) {
      try {
        setUploadProgress({ filename: file.name, pct: 0 })
        await uploadMut.mutateAsync({
          siteId,
          path: currentPath,
          file,
          onProgress: (pct) => setUploadProgress({ filename: file.name, pct }),
        })
        const ext = file.name.split('.').pop()?.toLowerCase() ?? ''
        if (ARCHIVE_EXTENSIONS.has(ext)) {
          const filePath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`
          lastZipPath = { path: filePath, name: file.name }
        }
      } catch {
        failed++
      }
    }
    setUploadProgress(null)
    if (failed === 0) {
      showMsg('success', `Uploaded ${fileArr.length} file(s)`)
      if (lastZipPath) setExtractPrompt(lastZipPath)
    } else {
      showMsg('error', `${failed} file(s) failed to upload`)
    }
  }

  // Extract archive
  const handleExtract = async (filePath: string, fileName: string) => {
    setExtractPrompt(null)
    setContextMenu(null)
    try {
      await extractMut.mutateAsync({ siteId, path: filePath })
      showMsg('success', `Extracted "${fileName}" successfully`)
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Extraction failed'
      showMsg('error', msg)
    }
  }

  // Drag & drop
  const handleDragOver = (e: React.DragEvent) => { e.preventDefault(); setIsDragging(true) }
  const handleDragLeave = () => setIsDragging(false)
  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setIsDragging(false)
    handleUpload(e.dataTransfer.files)
  }

  // Download
  const handleDownload = (file: FileInfo) => {
    setContextMenu(null)
    window.open(getFileDownloadUrl(siteId, file.path), '_blank')
  }

  // Search
  const handleSearch = async () => {
    if (!searchPattern.trim()) return
    try {
      const results = await searchMut.mutateAsync({ siteId, req: { path: currentPath, pattern: searchPattern } })
      setSearchResults(results)
    } catch {
      showMsg('error', 'Search failed')
    }
  }

  // If editing a file, show the editor
  if (editingFile) {
    return <FileEditor siteId={siteId} filePath={editingFile} onClose={() => setEditingFile(null)} />
  }

  const sortedFiles = files
    ? [...files].sort((a, b) => {
        if (a.is_dir && !b.is_dir) return -1
        if (!a.is_dir && b.is_dir) return 1
        return a.name.localeCompare(b.name)
      })
    : []

  const allSelected = sortedFiles.length > 0 && sortedFiles.every(f => selected.has(f.path))
  const someSelected = selected.size > 0

  return (
    <div onClick={closeContextMenu}>
      {/* Alert message */}
      {actionMsg && (
        <div className={`mb-3 px-4 py-2 rounded-lg text-sm font-medium ${
          actionMsg.type === 'success' ? 'bg-green-50 text-green-800 border border-green-200' : 'bg-red-50 text-red-800 border border-red-200'
        }`}>
          {actionMsg.text}
        </div>
      )}

      {/* Toolbar */}
      <div className="flex items-center justify-between mb-3 gap-2 flex-wrap">
        <div className="flex items-center gap-1">
          <button onClick={goUp} disabled={currentPath === '/'}
            className="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg disabled:opacity-30">
            <ArrowLeft size={16} />
          </button>
          <button onClick={() => navigateTo('/')}
            className="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg">
            <Home size={16} />
          </button>
          <button onClick={() => refetch()}
            className="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg">
            <RefreshCw size={16} />
          </button>
          <div className="h-5 w-px bg-gray-200 mx-1" />
          <button onClick={() => setShowNewFolder(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-100 rounded-lg">
            <FolderPlus size={15} /> New Folder
          </button>
          <button onClick={() => fileInputRef.current?.click()}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-100 rounded-lg">
            <Upload size={15} /> Upload
          </button>
          <input ref={fileInputRef} type="file" multiple className="hidden" onChange={(e) => handleUpload(e.target.files)} />
          {/* Paste button when clipboard has items */}
          {clipboard && (
            <button onClick={handlePaste}
              disabled={copyMut.isPending || moveMut.isPending}
              className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 rounded-lg">
              <Clipboard size={15} />
              Paste {clipboard.op === 'cut' ? '(Move)' : '(Copy)'} {clipboard.paths.length} item{clipboard.paths.length > 1 ? 's' : ''}
            </button>
          )}
          {clipboard && (
            <button onClick={() => setClipboard(null)}
              className="p-1.5 text-gray-400 hover:text-gray-600 rounded" title="Clear clipboard">
              <X size={14} />
            </button>
          )}
        </div>
        <div className="flex items-center gap-1">
          <button onClick={() => setShowSearch(!showSearch)}
            className={`flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg ${
              showSearch ? 'bg-blue-50 text-blue-700' : 'text-gray-700 hover:bg-gray-100'
            }`}>
            <Search size={15} /> Search
          </button>
        </div>
      </div>

      {/* Bulk action bar — shown when items selected */}
      {someSelected && (
        <div className="mb-3 flex items-center gap-2 px-4 py-2.5 bg-blue-50 border border-blue-200 rounded-lg">
          <CheckSquare size={16} className="text-blue-600 shrink-0" />
          <span className="text-sm font-medium text-blue-800">{selected.size} item(s) selected</span>
          <div className="flex-1" />
          <button onClick={handleCopySelected}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-gray-700 bg-white border border-gray-200 rounded-lg hover:bg-gray-50">
            <Copy size={13} /> Copy
          </button>
          <button onClick={handleCutSelected}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-gray-700 bg-white border border-gray-200 rounded-lg hover:bg-gray-50">
            <Scissors size={13} /> Cut
          </button>
          <button onClick={handleDeleteSelected}
            disabled={deleteBulkMut.isPending}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-white bg-red-600 rounded-lg hover:bg-red-700 disabled:opacity-50">
            <Trash2 size={13} /> Delete
          </button>
          <button onClick={clearSelection}
            className="p-1.5 text-blue-400 hover:text-blue-600 rounded">
            <X size={14} />
          </button>
        </div>
      )}

      {/* Search bar */}
      {showSearch && (
        <div className="mb-3 p-3 bg-gray-50 rounded-lg border border-gray-200">
          <div className="flex items-center gap-2">
            <input type="text" value={searchPattern} onChange={(e) => setSearchPattern(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
              placeholder="Search file contents (grep)..."
              className="flex-1 px-3 py-1.5 text-sm border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent" />
            <button onClick={handleSearch} disabled={searchMut.isPending}
              className="px-4 py-1.5 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
              {searchMut.isPending ? 'Searching...' : 'Search'}
            </button>
            <button onClick={() => { setShowSearch(false); setSearchResults([]) }}
              className="p-1.5 text-gray-400 hover:text-gray-600 rounded">
              <X size={16} />
            </button>
          </div>
          {searchResults.length > 0 && (
            <div className="mt-3 max-h-64 overflow-auto">
              <p className="text-xs text-gray-500 mb-2">Found {searchResults.length} result(s)</p>
              {searchResults.map((r, i) => (
                <div key={i} onClick={() => setEditingFile(r.file)}
                  className="px-3 py-2 text-sm hover:bg-white rounded cursor-pointer border-b border-gray-100 last:border-0">
                  <span className="text-blue-600 font-mono text-xs">{r.file}:{r.line}</span>
                  <pre className="text-gray-600 mt-0.5 text-xs whitespace-pre-wrap">{r.content}</pre>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Breadcrumb */}
      <div className="flex items-center gap-1 px-3 py-2 mb-3 bg-gray-50 rounded-lg text-sm overflow-x-auto">
        <button onClick={() => navigateTo('/')} className="text-blue-600 hover:underline shrink-0">root</button>
        {pathParts.map((part, i) => {
          const path = '/' + pathParts.slice(0, i + 1).join('/')
          return (
            <span key={path} className="flex items-center gap-1 shrink-0">
              <ChevronRight size={14} className="text-gray-400" />
              <button onClick={() => navigateTo(path)} className="text-blue-600 hover:underline">
                {part}
              </button>
            </span>
          )
        })}
      </div>

      {/* New folder inline input */}
      {showNewFolder && (
        <div className="mb-3 flex items-center gap-2 px-3 py-2 bg-yellow-50 border border-yellow-200 rounded-lg">
          <FolderPlus size={16} className="text-yellow-600" />
          <input type="text" value={newFolderName} onChange={(e) => setNewFolderName(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') handleCreateFolder(); if (e.key === 'Escape') setShowNewFolder(false) }}
            placeholder="New folder name..."
            className="flex-1 px-2 py-1 text-sm border border-yellow-300 rounded focus:outline-none focus:ring-1 focus:ring-yellow-500"
            autoFocus />
          <button onClick={handleCreateFolder} disabled={mkdirMut.isPending}
            className="px-3 py-1 text-sm text-white bg-yellow-600 rounded hover:bg-yellow-700 disabled:opacity-50">
            Create
          </button>
          <button onClick={() => { setShowNewFolder(false); setNewFolderName('') }}
            className="p-1 text-gray-400 hover:text-gray-600">
            <X size={16} />
          </button>
        </div>
      )}

      {/* Upload progress bar */}
      {uploadProgress && (
        <div className="mb-3 px-4 py-3 bg-blue-50 border border-blue-200 rounded-lg">
          <div className="flex items-center justify-between mb-1.5">
            <div className="flex items-center gap-2 text-sm text-blue-700">
              <div className="animate-spin rounded-full h-3.5 w-3.5 border-b-2 border-blue-600" />
              <span className="font-medium truncate max-w-xs">{uploadProgress.filename}</span>
            </div>
            <span className="text-xs font-semibold text-blue-700">{uploadProgress.pct}%</span>
          </div>
          <div className="w-full bg-blue-100 rounded-full h-1.5">
            <div className="bg-blue-500 h-1.5 rounded-full transition-all duration-300"
              style={{ width: `${uploadProgress.pct}%` }} />
          </div>
        </div>
      )}

      {/* Extract prompt after zip upload */}
      {extractPrompt && (
        <div className="mb-3 flex items-center gap-3 px-4 py-3 bg-amber-50 border border-amber-200 rounded-lg">
          <Archive size={16} className="text-amber-600 shrink-0" />
          <div className="flex-1 text-sm text-amber-800">
            <span className="font-medium">"{extractPrompt.name}"</span> uploaded. Extract it now?
          </div>
          <button onClick={() => handleExtract(extractPrompt.path, extractPrompt.name)}
            disabled={extractMut.isPending}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-white bg-amber-600 rounded-lg hover:bg-amber-700 disabled:opacity-50 shrink-0">
            <PackageOpen size={12} /> Extract Here
          </button>
          <button onClick={() => setExtractPrompt(null)} className="p-1 text-amber-400 hover:text-amber-600">
            <X size={14} />
          </button>
        </div>
      )}

      {/* Extract in progress */}
      {extractMut.isPending && !extractPrompt && (
        <div className="mb-3 flex items-center gap-2 px-4 py-2 bg-amber-50 border border-amber-200 rounded-lg text-sm text-amber-700">
          <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-amber-600" />
          Extracting...
        </div>
      )}

      {/* File table */}
      <div
        className={`bg-white rounded-xl border ${isDragging ? 'border-blue-400 bg-blue-50' : 'border-gray-200'} overflow-hidden`}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        {isDragging && (
          <div className="p-8 text-center text-blue-600">
            <Upload size={32} className="mx-auto mb-2" />
            <p className="text-sm font-medium">Drop files here to upload</p>
          </div>
        )}
        {!isDragging && (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-100 text-left text-gray-500 text-xs uppercase">
                {/* Select-all checkbox */}
                <th className="pl-4 pr-2 py-2.5 w-8">
                  <button onClick={(e) => { e.stopPropagation(); allSelected ? clearSelection() : selectAll() }}
                    className="text-gray-400 hover:text-blue-600">
                    {allSelected
                      ? <CheckSquare size={15} className="text-blue-600" />
                      : <Square size={15} />}
                  </button>
                </th>
                <th className="px-2 py-2.5 font-medium">Name</th>
                <th className="px-4 py-2.5 font-medium w-24 hidden sm:table-cell">Size</th>
                <th className="px-4 py-2.5 font-medium w-28 hidden md:table-cell">Permissions</th>
                <th className="px-4 py-2.5 font-medium w-20 hidden md:table-cell">Owner</th>
                <th className="px-4 py-2.5 font-medium w-40 hidden lg:table-cell">Modified</th>
                <th className="px-4 py-2.5 font-medium w-10"></th>
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <tr>
                  <td colSpan={7} className="px-4 py-12 text-center text-gray-400">
                    <div className="flex items-center justify-center gap-2">
                      <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-600" />
                      Loading...
                    </div>
                  </td>
                </tr>
              ) : sortedFiles.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-12 text-center text-gray-400">Empty folder</td>
                </tr>
              ) : (
                sortedFiles.map((file) => {
                  const isSelected = selected.has(file.path)
                  const isCut = clipboard?.op === 'cut' && clipboard.paths.includes(file.path)
                  return (
                    <tr key={file.path}
                      className={`border-b border-gray-50 cursor-pointer group ${
                        isSelected ? 'bg-blue-50 hover:bg-blue-100' : 'hover:bg-gray-50'
                      } ${isCut ? 'opacity-50' : ''}`}
                      onClick={(e) => handleFileClick(file, e)}
                      onContextMenu={(e) => handleContextMenu(e, file)}
                    >
                      {/* Checkbox */}
                      <td className="pl-4 pr-2 py-2" onClick={(e) => handleCheckboxClick(e, file.path)}>
                        <div className={`${isSelected ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'} transition-opacity`}>
                          {isSelected
                            ? <CheckSquare size={15} className="text-blue-600" />
                            : <Square size={15} className="text-gray-400" />}
                        </div>
                      </td>
                      <td className="px-2 py-2">
                        {renaming?.path === file.path ? (
                          <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
                            {getFileIcon(file)}
                            <input type="text" value={renameValue} onChange={(e) => setRenameValue(e.target.value)}
                              onKeyDown={(e) => { if (e.key === 'Enter') handleRename(); if (e.key === 'Escape') setRenaming(null) }}
                              className="flex-1 px-2 py-0.5 text-sm border border-blue-300 rounded focus:outline-none focus:ring-1 focus:ring-blue-500"
                              autoFocus />
                            <button onClick={handleRename} className="text-blue-600 hover:text-blue-800"><Save size={14} /></button>
                            <button onClick={() => setRenaming(null)} className="text-gray-400 hover:text-gray-600"><X size={14} /></button>
                          </div>
                        ) : (
                          <div className="flex items-center gap-2">
                            {getFileIcon(file)}
                            <span className={`${file.is_dir ? 'font-medium text-gray-900' : 'text-gray-700'}`}>
                              {file.name}
                            </span>
                            {isCut && <span className="text-xs text-gray-400 italic">(cut)</span>}
                          </div>
                        )}
                      </td>
                      <td className="px-4 py-2 text-gray-500 hidden sm:table-cell">
                        {file.is_dir ? '-' : formatBytes(file.size)}
                      </td>
                      <td className="px-4 py-2 hidden md:table-cell">
                        <code className="text-xs bg-gray-100 px-1.5 py-0.5 rounded text-gray-600">{file.permissions}</code>
                      </td>
                      <td className="px-4 py-2 text-gray-500 hidden md:table-cell text-xs">{file.owner}</td>
                      <td className="px-4 py-2 text-gray-500 text-xs hidden lg:table-cell">{formatDate(file.mod_time)}</td>
                      <td className="px-4 py-2" onClick={(e) => e.stopPropagation()}>
                        <button onClick={(e) => handleContextMenu(e, file)}
                          className="p-1 text-gray-400 hover:text-gray-600 rounded opacity-0 group-hover:opacity-100">
                          <MoreVertical size={16} />
                        </button>
                      </td>
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        )}
      </div>

      {/* Context Menu */}
      {contextMenu && (() => {
        const isArchive = !contextMenu.file.is_dir && isArchiveFile(contextMenu.file.name)
        const ctxSelected = selected.size > 1 ? selected : new Set([contextMenu.file.path])
        return (
          <div className="fixed z-50 bg-white rounded-lg shadow-lg border border-gray-200 py-1 min-w-[180px]"
            style={{ left: contextMenu.x, top: contextMenu.y }}>
            {/* Single file actions */}
            {selected.size <= 1 && !contextMenu.file.is_dir && isTextFile(contextMenu.file.name) && (
              <CtxMenuItem icon={<Edit3 size={14} />} label="Edit"
                onClick={() => { setEditingFile(contextMenu.file.path); setContextMenu(null) }} />
            )}
            {selected.size <= 1 && isArchive && (
              <CtxMenuItem icon={<PackageOpen size={14} />} label="Extract Here"
                onClick={() => handleExtract(contextMenu.file.path, contextMenu.file.name)} />
            )}
            {selected.size <= 1 && (
              <CtxMenuItem icon={<Download size={14} />} label="Download"
                onClick={() => handleDownload(contextMenu.file)} />
            )}
            {selected.size <= 1 && (
              <CtxMenuItem icon={<Edit3 size={14} />} label="Rename"
                onClick={() => { setRenaming(contextMenu.file); setRenameValue(contextMenu.file.name); setContextMenu(null) }} />
            )}
            {selected.size <= 1 && (
              <CtxMenuItem icon={<Lock size={14} />} label="Permissions (chmod)"
                onClick={() => { setShowChmod(contextMenu.file); setChmodValue(contextMenu.file.permissions.replace(/[^\d]/g, '').slice(-3)); setContextMenu(null) }} />
            )}
            <div className="border-t border-gray-100 my-1" />
            {/* Multi-select friendly actions */}
            <CtxMenuItem icon={<Copy size={14} />} label={`Copy${ctxSelected.size > 1 ? ` (${ctxSelected.size})` : ''}`}
              onClick={() => { setSelected(ctxSelected); setClipboard({ paths: Array.from(ctxSelected), op: 'copy' }); setContextMenu(null); showMsg('success', `${ctxSelected.size} item(s) copied`) }} />
            <CtxMenuItem icon={<Scissors size={14} />} label={`Cut${ctxSelected.size > 1 ? ` (${ctxSelected.size})` : ''}`}
              onClick={() => { setSelected(ctxSelected); setClipboard({ paths: Array.from(ctxSelected), op: 'cut' }); setContextMenu(null); showMsg('success', `${ctxSelected.size} item(s) cut`) }} />
            {clipboard && (
              <CtxMenuItem icon={<Clipboard size={14} />} label="Paste Here"
                onClick={handlePaste} />
            )}
            <div className="border-t border-gray-100 my-1" />
            <CtxMenuItem icon={<Trash2 size={14} />} label={`Delete${ctxSelected.size > 1 ? ` (${ctxSelected.size})` : ''}`} danger
              onClick={() => ctxSelected.size > 1
                ? handleDeleteSelected()
                : handleDelete(contextMenu.file)} />
          </div>
        )
      })()}

      {/* Chmod Modal */}
      {showChmod && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowChmod(null)}>
          <div className="bg-white rounded-xl p-6 w-full max-w-sm shadow-xl" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-lg font-semibold mb-1">Change Permissions</h3>
            <p className="text-sm text-gray-500 mb-4">{showChmod.name}</p>
            <div className="space-y-3">
              <div>
                <label className="text-sm text-gray-600 block mb-1">Permissions (e.g. 755, 644)</label>
                <input type="text" value={chmodValue} onChange={(e) => setChmodValue(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && handleChmod()}
                  placeholder="755"
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                  maxLength={4} autoFocus />
              </div>
              {showChmod.is_dir && (
                <label className="flex items-center gap-2 text-sm text-gray-600">
                  <input type="checkbox" checked={chmodRecursive} onChange={(e) => setChmodRecursive(e.target.checked)}
                    className="rounded border-gray-300" />
                  Apply recursively
                </label>
              )}
            </div>
            <div className="flex gap-3 mt-5">
              <button onClick={() => setShowChmod(null)}
                className="flex-1 px-4 py-2 text-sm text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200">
                Cancel
              </button>
              <button onClick={handleChmod} disabled={chmodMut.isPending}
                className="flex-1 px-4 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {chmodMut.isPending ? 'Updating...' : 'Apply'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function CtxMenuItem({ icon, label, onClick, danger }: {
  icon: React.ReactNode; label: string; onClick: () => void; danger?: boolean
}) {
  return (
    <button onClick={onClick}
      className={`w-full flex items-center gap-2 px-3 py-1.5 text-sm hover:bg-gray-50 ${
        danger ? 'text-red-600 hover:bg-red-50' : 'text-gray-700'
      }`}>
      {icon} {label}
    </button>
  )
}
