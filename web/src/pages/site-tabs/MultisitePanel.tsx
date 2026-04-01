import { useState } from 'react'
import { Loader2, Network, RefreshCw, Plus } from 'lucide-react'
import { useSubsites, useEnableMultisite, useCreateSubsite, useDeleteSubsite, useSyncSubsites } from '@/hooks/useMultisite'
import type { Subsite } from '@/types'

export function MultisitePanel({ siteId, isMultisite, canManage }: { siteId: number; isMultisite: boolean; canManage: boolean }) {
  const { data: subsites, isLoading } = useSubsites(siteId, isMultisite)
  const enableMultisite = useEnableMultisite(siteId)
  const createSubsite = useCreateSubsite(siteId)
  const deleteSubsite = useDeleteSubsite(siteId)
  const syncSubsites = useSyncSubsites(siteId)

  const [msType, setMsType] = useState<'subdomain' | 'subfolder'>('subdomain')
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [newSlug, setNewSlug] = useState('')
  const [newTitle, setNewTitle] = useState('')

  const handleEnable = async () => {
    if (!confirm(`Enable WordPress Multisite (${msType} mode)? This cannot be undone.`)) return
    try {
      await enableMultisite.mutateAsync({ type: msType })
    } catch {}
  }

  const handleCreate = async () => {
    if (!newSlug || !newTitle) return
    try {
      await createSubsite.mutateAsync({ slug: newSlug, title: newTitle })
      setNewSlug('')
      setNewTitle('')
      setShowCreateForm(false)
    } catch {}
  }

  const handleDelete = async (subsiteId: number, domain: string) => {
    if (!confirm(`Delete subsite "${domain}"? This cannot be undone.`)) return
    try {
      await deleteSubsite.mutateAsync(subsiteId)
    } catch {}
  }

  // Not multisite yet — show enable form
  if (!isMultisite) {
    return (
      <div className="bg-white rounded-xl border border-gray-200 p-8">
        <div className="max-w-lg mx-auto text-center">
          <Network size={48} className="mx-auto text-gray-400 mb-4" />
          <h3 className="text-lg font-semibold mb-2">Enable WordPress Multisite</h3>
          <p className="text-sm text-gray-500 mb-6">
            Transform this WordPress installation into a multisite network. This allows you to manage multiple sites from a single WordPress dashboard.
          </p>
          {canManage && (
            <div className="space-y-4">
              <div className="flex items-center justify-center gap-4">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="radio" name="msType" value="subdomain" checked={msType === 'subdomain'}
                    onChange={() => setMsType('subdomain')} className="text-blue-600" />
                  <span className="text-sm font-medium">Subdomain</span>
                  <span className="text-xs text-gray-400">(sub.domain.com)</span>
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="radio" name="msType" value="subfolder" checked={msType === 'subfolder'}
                    onChange={() => setMsType('subfolder')} className="text-blue-600" />
                  <span className="text-sm font-medium">Subfolder</span>
                  <span className="text-xs text-gray-400">(domain.com/sub/)</span>
                </label>
              </div>
              <button onClick={handleEnable} disabled={enableMultisite.isPending}
                className="inline-flex items-center gap-2 px-6 py-2.5 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {enableMultisite.isPending ? <Loader2 size={16} className="animate-spin" /> : <Network size={16} />}
                Enable Multisite
              </button>
              {enableMultisite.isSuccess && (
                <p className="text-green-600 text-sm">Multisite enable task queued! Refresh after the agent completes the task.</p>
              )}
            </div>
          )}
        </div>
      </div>
    )
  }

  // Multisite enabled — show subsites management
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold">Subsites</h3>
        <div className="flex gap-2">
          {canManage && (
            <>
              <button onClick={() => syncSubsites.mutate()} disabled={syncSubsites.isPending}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm border border-gray-300 rounded-lg hover:bg-gray-50 disabled:opacity-50">
                {syncSubsites.isPending ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
                Sync
              </button>
              <button onClick={() => setShowCreateForm(!showCreateForm)}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700">
                <Plus size={14} /> Add Subsite
              </button>
            </>
          )}
        </div>
      </div>

      {/* Create form */}
      {showCreateForm && (
        <div className="bg-white rounded-xl border border-gray-200 p-4">
          <h4 className="text-sm font-semibold mb-3">Create New Subsite</h4>
          <div className="flex gap-3 items-end">
            <div className="flex-1">
              <label className="block text-xs text-gray-500 mb-1">Slug</label>
              <input type="text" value={newSlug} onChange={(e) => setNewSlug(e.target.value)}
                placeholder="blog" className="w-full px-3 py-2 border rounded-lg text-sm" />
            </div>
            <div className="flex-1">
              <label className="block text-xs text-gray-500 mb-1">Title</label>
              <input type="text" value={newTitle} onChange={(e) => setNewTitle(e.target.value)}
                placeholder="My Blog" className="w-full px-3 py-2 border rounded-lg text-sm" />
            </div>
            <button onClick={handleCreate} disabled={createSubsite.isPending || !newSlug || !newTitle}
              className="px-4 py-2 bg-green-600 text-white text-sm rounded-lg hover:bg-green-700 disabled:opacity-50">
              {createSubsite.isPending ? <Loader2 size={14} className="animate-spin" /> : 'Create'}
            </button>
            <button onClick={() => setShowCreateForm(false)}
              className="px-4 py-2 text-sm border border-gray-300 rounded-lg hover:bg-gray-50">Cancel</button>
          </div>
        </div>
      )}

      {/* Subsites table */}
      <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <Loader2 size={24} className="animate-spin text-gray-400" />
          </div>
        ) : !subsites || subsites.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            <Network size={32} className="mx-auto mb-2 text-gray-300" />
            <p className="text-sm">No subsites yet. Click "Add Subsite" to create one.</p>
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-gray-50">
                <th className="text-left px-4 py-3 font-medium text-gray-600">Blog ID</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Domain / Path</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Title</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Status</th>
                {canManage && <th className="text-right px-4 py-3 font-medium text-gray-600">Actions</th>}
              </tr>
            </thead>
            <tbody className="divide-y">
              {subsites.map((sub: Subsite) => (
                <tr key={sub.id} className="hover:bg-gray-50">
                  <td className="px-4 py-3 text-gray-700">{sub.wp_blog_id}</td>
                  <td className="px-4 py-3">
                    <span className="font-mono text-xs">{sub.domain}{sub.path}</span>
                  </td>
                  <td className="px-4 py-3 text-gray-700">{sub.title || '-'}</td>
                  <td className="px-4 py-3">
                    <span className={`inline-flex px-2 py-0.5 text-xs font-medium rounded-full ${
                      sub.status === 'active' ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-600'
                    }`}>{sub.status}</span>
                  </td>
                  {canManage && (
                    <td className="px-4 py-3 text-right">
                      <button onClick={() => handleDelete(sub.id, sub.domain + sub.path)}
                        disabled={deleteSubsite.isPending}
                        className="text-red-600 hover:text-red-800 text-xs font-medium">
                        Delete
                      </button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
