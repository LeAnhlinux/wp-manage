import { useState } from 'react'
import { Loader2, Code, AlertTriangle, Play } from 'lucide-react'
import { useWPCLIHistory, useExecuteWPCLI } from '@/hooks/useWPCLI'

export function WPCLIPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: history = [], isLoading } = useWPCLIHistory(siteId)
  const executeCmd = useExecuteWPCLI(siteId)
  const [command, setCommand] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!command.trim()) return
    await executeCmd.mutateAsync({ command: command.trim() })
    setCommand('')
  }

  return (
    <div className="space-y-4">
      <div className="bg-gray-900 rounded-xl p-6">
        <div className="flex items-center gap-2 mb-4">
          <Code size={18} className="text-green-400" />
          <h2 className="text-lg font-semibold text-white">WP-CLI Terminal</h2>
        </div>

        {/* Command output area */}
        <div className="bg-black rounded-lg p-4 mb-4 max-h-96 overflow-y-auto font-mono text-sm space-y-3">
          {isLoading ? (
            <p className="text-gray-500">Loading history...</p>
          ) : history.length === 0 ? (
            <p className="text-gray-500">No commands executed yet. Type a WP-CLI command below.</p>
          ) : (
            [...history].reverse().map((entry) => (
              <div key={entry.id} className="border-b border-gray-800 pb-2">
                <div className="flex items-center gap-2">
                  <span className="text-green-400">$</span>
                  <span className="text-blue-300">wp {entry.command}</span>
                  <span className="ml-auto text-xs text-gray-600">{new Date(entry.created_at).toLocaleString()}</span>
                </div>
                {entry.status === 'running' ? (
                  <div className="flex items-center gap-2 mt-1 text-yellow-400 text-xs">
                    <Loader2 size={12} className="animate-spin" /> Running...
                  </div>
                ) : (
                  <pre className={`mt-1 whitespace-pre-wrap text-xs ${entry.exit_code === 0 ? 'text-gray-300' : 'text-red-400'}`}>
                    {entry.output || '(no output)'}
                  </pre>
                )}
              </div>
            ))
          )}
        </div>

        {/* Command input */}
        {canManage && (
          <form onSubmit={handleSubmit} className="flex items-center gap-2">
            <span className="text-green-400 font-mono text-sm">wp</span>
            <input
              type="text"
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              placeholder="plugin list, cache flush, option get siteurl..."
              className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm text-white font-mono placeholder-gray-500 focus:outline-none focus:border-green-500"
              disabled={executeCmd.isPending}
            />
            <button
              type="submit"
              disabled={executeCmd.isPending || !command.trim()}
              className="px-4 py-2 bg-green-600 text-white rounded-lg text-sm hover:bg-green-700 disabled:opacity-50 flex items-center gap-1.5"
            >
              {executeCmd.isPending ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
              Run
            </button>
          </form>
        )}
      </div>

      <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
        <div className="flex items-center gap-2 text-yellow-800 mb-2">
          <AlertTriangle size={16} />
          <p className="text-sm font-medium">Security Notice</p>
        </div>
        <p className="text-xs text-yellow-700">
          Commands like <code className="bg-yellow-100 px-1 rounded">eval</code>, <code className="bg-yellow-100 px-1 rounded">shell</code>, <code className="bg-yellow-100 px-1 rounded">db export</code>, and <code className="bg-yellow-100 px-1 rounded">db import</code> are blocked for security reasons.
          The <code className="bg-yellow-100 px-1 rounded">wp</code> prefix is added automatically.
        </p>
      </div>
    </div>
  )
}
