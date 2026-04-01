import { useState } from 'react'
import { Key, Plus, Check, Eye, EyeOff, Trash2 } from 'lucide-react'
import { useSFTPAccounts, useCreateSFTP, useDeleteSFTP } from '@/hooks/useSFTP'
import { CopyButton } from './shared'

export function SFTPPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: accounts = [], isLoading } = useSFTPAccounts(siteId)
  const createSFTP = useCreateSFTP(siteId)
  const deleteSFTP = useDeleteSFTP(siteId)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ username: '', password: '' })
  const [showPassword, setShowPassword] = useState(false)
  const [showCredPassword, setShowCredPassword] = useState(false)
  const [newCreds, setNewCreds] = useState<{ host: string; port: number; username: string; password: string } | null>(null)
  const [createError, setCreateError] = useState<string | null>(null)

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreateError(null)
    try {
      const result = await createSFTP.mutateAsync(form)
      setNewCreds({
        host: result.host || '—',
        port: result.port || 22,
        username: result.username,
        password: form.password,
      })
      setForm({ username: '', password: '' })
      setShowForm(false)
      setShowCredPassword(false)
    } catch (err: any) {
      setCreateError(err?.response?.data?.error || 'Failed to create SFTP account')
    }
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
          <Key size={20} className="text-purple-500" /> SFTP Accounts
        </h2>
        {canManage && (
          <button onClick={() => { setShowForm(!showForm); setCreateError(null) }}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
            <Plus size={14} /> Create Account
          </button>
        )}
      </div>

      <p className="text-sm text-gray-500 mb-4">
        SFTP accounts provide chrooted file access to this site. Users are restricted to the site directory. Port: <span className="font-mono font-medium">22</span>.
      </p>

      {/* Credential Card — shown once after creation */}
      {newCreds && (
        <div className="mb-4 p-4 bg-green-50 border border-green-200 rounded-xl space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-green-800 flex items-center gap-2">
              <Check size={15} className="text-green-600" /> SFTP Account Created — Save these credentials!
            </h3>
            <button onClick={() => setNewCreds(null)} className="text-xs text-green-600 hover:text-green-800 underline">Dismiss</button>
          </div>
          <p className="text-xs text-green-600">⚠️ The password will not be shown again. Copy it now.</p>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-sm">
            {[
              { label: 'Host', value: newCreds.host },
              { label: 'Port', value: String(newCreds.port) },
              { label: 'Username', value: newCreds.username },
            ].map(({ label, value }) => (
              <div key={label} className="flex items-center gap-2 bg-white border border-green-200 rounded-lg px-3 py-2">
                <span className="text-xs text-gray-500 w-20 flex-shrink-0">{label}</span>
                <span className="font-mono text-gray-900 flex-1 truncate">{value}</span>
                <CopyButton value={value} />
              </div>
            ))}
            <div className="flex items-center gap-2 bg-white border border-green-200 rounded-lg px-3 py-2">
              <span className="text-xs text-gray-500 w-20 flex-shrink-0">Password</span>
              <span className="font-mono text-gray-900 flex-1 truncate">
                {showCredPassword ? newCreds.password : '••••••••••••'}
              </span>
              <button onClick={() => setShowCredPassword(v => !v)} className="p-1 text-gray-400 hover:text-gray-600 flex-shrink-0">
                {showCredPassword ? <EyeOff size={13} /> : <Eye size={13} />}
              </button>
              <CopyButton value={newCreds.password} />
            </div>
          </div>
          <div className="text-xs text-green-700 font-mono bg-green-100 rounded px-3 py-2">
            sftp {newCreds.username}@{newCreds.host}
          </div>
        </div>
      )}

      {showForm && (
        <form onSubmit={handleCreate} className="bg-gray-50 rounded-lg p-4 mb-4 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Username</label>
              <input value={form.username} onChange={e => setForm({ ...form, username: e.target.value })}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm"
                placeholder="sftp_mysite" required />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Password</label>
              <div className="relative">
                <input type={showPassword ? 'text' : 'password'}
                  value={form.password} onChange={e => setForm({ ...form, password: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm pr-10"
                  placeholder="Strong password" required />
                <button type="button" onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-gray-400 hover:text-gray-600">
                  {showPassword ? <EyeOff size={14} /> : <Eye size={14} />}
                </button>
              </div>
            </div>
          </div>
          {createError && (
            <p className="text-xs text-red-600 bg-red-50 border border-red-200 rounded px-3 py-2">{createError}</p>
          )}
          <div className="flex gap-2">
            <button type="submit" disabled={createSFTP.isPending}
              className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
              {createSFTP.isPending ? 'Creating...' : 'Create'}
            </button>
            <button type="button" onClick={() => { setShowForm(false); setCreateError(null) }}
              className="px-4 py-2 text-sm bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300">
              Cancel
            </button>
          </div>
        </form>
      )}

      {isLoading ? (
        <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
      ) : accounts.length === 0 ? (
        <p className="text-sm text-gray-400 text-center py-8">No SFTP accounts configured</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-gray-500 border-b">
                <th className="pb-2 font-medium">Username</th>
                <th className="pb-2 font-medium">Home Directory</th>
                <th className="pb-2 font-medium">Created</th>
                <th className="pb-2 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {accounts.map(acc => (
                <tr key={acc.id} className="border-b border-gray-50">
                  <td className="py-3 font-mono font-medium">{acc.username}</td>
                  <td className="py-3 font-mono text-gray-500 text-xs">{acc.home_dir}</td>
                  <td className="py-3 text-gray-500">{new Date(acc.created_at).toLocaleDateString()}</td>
                  <td className="py-3">
                    {canManage && (
                      <button onClick={() => { if (confirm(`Delete SFTP account "${acc.username}"?`)) deleteSFTP.mutate(acc.id) }}
                        className="p-1 text-gray-400 hover:text-red-600">
                        <Trash2 size={14} />
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
