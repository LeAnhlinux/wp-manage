import { useState, useEffect } from 'react'
import { ArrowLeft, Save, X, FileText } from 'lucide-react'
import { useFileContent, useSaveFileContent } from '@/hooks/useFiles'

interface Props {
  siteId: number
  filePath: string
  onClose: () => void
}

export default function FileEditor({ siteId, filePath, onClose }: Props) {
  const { data, isLoading, error } = useFileContent(siteId, filePath, true)
  const saveContent = useSaveFileContent()
  const [content, setContent] = useState('')
  const [hasChanges, setHasChanges] = useState(false)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (data?.content !== undefined) {
      setContent(data.content)
      setHasChanges(false)
    }
  }, [data?.content])

  const handleSave = async () => {
    try {
      await saveContent.mutateAsync({ siteId, req: { path: filePath, content } })
      setHasChanges(false)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch {
      alert('Failed to save file')
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      e.preventDefault()
      handleSave()
    }
  }

  const fileName = filePath.split('/').pop() || filePath

  const handleClose = () => {
    if (hasChanges && !confirm('You have unsaved changes. Close anyway?')) return
    onClose()
  }

  return (
    <div className="flex flex-col h-[calc(100vh-200px)]">
      {/* Editor Header */}
      <div className="flex items-center justify-between px-4 py-2 bg-gray-800 rounded-t-xl">
        <div className="flex items-center gap-3">
          <button onClick={handleClose} className="p-1 text-gray-400 hover:text-white rounded">
            <ArrowLeft size={16} />
          </button>
          <div className="flex items-center gap-2 text-white">
            <FileText size={16} className="text-blue-400" />
            <span className="text-sm font-medium">{fileName}</span>
            {hasChanges && <span className="w-2 h-2 bg-orange-400 rounded-full" />}
          </div>
          <span className="text-xs text-gray-500 font-mono">{filePath}</span>
        </div>
        <div className="flex items-center gap-2">
          {saved && (
            <span className="text-xs text-green-400">Saved!</span>
          )}
          <button onClick={handleSave} disabled={saveContent.isPending || !hasChanges}
            className="flex items-center gap-1.5 px-3 py-1 text-sm text-white bg-blue-600 rounded hover:bg-blue-700 disabled:opacity-40">
            <Save size={14} />
            {saveContent.isPending ? 'Saving...' : 'Save'}
          </button>
          <span className="text-xs text-gray-500">Ctrl+S</span>
          <button onClick={handleClose} className="p-1 text-gray-400 hover:text-white rounded">
            <X size={16} />
          </button>
        </div>
      </div>

      {/* Editor Body */}
      <div className="flex-1 relative">
        {isLoading ? (
          <div className="flex items-center justify-center h-full bg-gray-900">
            <div className="flex items-center gap-2 text-gray-400">
              <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-500" />
              Loading file...
            </div>
          </div>
        ) : error ? (
          <div className="flex items-center justify-center h-full bg-gray-900">
            <p className="text-red-400 text-sm">Cannot read file: {(error as Error).message}</p>
          </div>
        ) : (
          <textarea
            value={content}
            onChange={(e) => { setContent(e.target.value); setHasChanges(true) }}
            onKeyDown={handleKeyDown}
            className="w-full h-full p-4 bg-gray-900 text-gray-100 font-mono text-sm resize-none focus:outline-none leading-6 tab-size-4"
            style={{ tabSize: 4 }}
            spellCheck={false}
          />
        )}
      </div>

      {/* Status bar */}
      <div className="flex items-center justify-between px-4 py-1 bg-gray-800 rounded-b-xl text-xs text-gray-500">
        <span>{getLanguage(fileName)}</span>
        <span>
          {content.split('\n').length} lines |{' '}
          {new Blob([content]).size > 1024
            ? `${(new Blob([content]).size / 1024).toFixed(1)} KB`
            : `${new Blob([content]).size} B`}
        </span>
      </div>
    </div>
  )
}

function getLanguage(fileName: string): string {
  const ext = fileName.split('.').pop()?.toLowerCase()
  const map: Record<string, string> = {
    php: 'PHP', js: 'JavaScript', ts: 'TypeScript', jsx: 'React JSX', tsx: 'React TSX',
    css: 'CSS', scss: 'SCSS', less: 'LESS', html: 'HTML', htm: 'HTML',
    json: 'JSON', xml: 'XML', yml: 'YAML', yaml: 'YAML', md: 'Markdown',
    sql: 'SQL', sh: 'Shell', bash: 'Bash', py: 'Python', rb: 'Ruby',
    conf: 'Config', ini: 'INI', env: 'Env', log: 'Log', txt: 'Text',
    htaccess: 'Apache', svg: 'SVG', tpl: 'Template', twig: 'Twig', vue: 'Vue',
  }
  return map[ext || ''] || 'Plain Text'
}
