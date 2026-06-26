import { useState } from 'react'
import { Plus, Trash2, Edit2, Server, X, KeyRound, Eye, EyeOff } from 'lucide-react'
import { useUsers, useCreateUser, useUpdateUser, useDeleteUser, useAssignServer, useUnassignServer, useUserServers } from '@/hooks/useUsers'
import { useServers } from '@/hooks/useServers'
import { StatusBadge } from '@/components/ui/Badge'

const ROLES = ['admin', 'manager', 'viewer'] as const

function RoleBadge({ role }: { role: string }) {
  const colors: Record<string, string> = {
    admin: 'bg-red-100 text-red-700',
    manager: 'bg-blue-100 text-blue-700',
    viewer: 'bg-gray-100 text-gray-600',
  }
  return (
    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium capitalize ${colors[role] || colors.viewer}`}>
      {role}
    </span>
  )
}

export default function UsersPage() {
  const [showCreate, setShowCreate] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [managingServersId, setManagingServersId] = useState<number | null>(null)
  const [changingPasswordId, setChangingPasswordId] = useState<number | null>(null)
  const [form, setForm] = useState({ email: '', password: '', name: '', role: 'viewer' as const })
  const [editForm, setEditForm] = useState({ name: '', email: '', role: 'viewer' as string, password: '' })
  const [createError, setCreateError] = useState('')
  const [editError, setEditError] = useState('')

  const { data, isLoading } = useUsers()
  const createUser = useCreateUser()
  const updateUser = useUpdateUser()
  const deleteUser = useDeleteUser()

  const users = data?.data || []

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreateError('')
    try {
      await createUser.mutateAsync(form)
      setShowCreate(false)
      setForm({ email: '', password: '', name: '', role: 'viewer' })
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error
      setCreateError(msg || 'Failed to create user')
    }
  }

  const startEdit = (user: { id: number; name: string; email: string; role: string }) => {
    setEditingId(user.id)
    setEditError('')
    setEditForm({ name: user.name, email: user.email, role: user.role, password: '' })
  }

  const handleUpdate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!editingId) return
    setEditError('')
    try {
      const updates: Record<string, string> = {}
      if (editForm.name) updates.name = editForm.name
      if (editForm.email) updates.email = editForm.email
      if (editForm.role) updates.role = editForm.role
      if (editForm.password) updates.password = editForm.password
      await updateUser.mutateAsync({ id: editingId, ...updates } as never)
      setEditingId(null)
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error
      setEditError(msg || 'Failed to update user')
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Users</h1>
        <button onClick={() => { setShowCreate(!showCreate); setCreateError('') }}
          className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700">
          <Plus size={18} /> Add User
        </button>
      </div>

      {/* Create Form */}
      {showCreate && (
        <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6 mb-6">
          <h2 className="text-lg font-semibold mb-4">Create New User</h2>
          <form onSubmit={handleCreate} className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
                <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })}
                  className="w-full px-4 py-2 border border-gray-300 rounded-lg" placeholder="John Doe" required />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Email</label>
                <input type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })}
                  className="w-full px-4 py-2 border border-gray-300 rounded-lg" placeholder="john@example.com" required />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Password</label>
                <input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })}
                  className="w-full px-4 py-2 border border-gray-300 rounded-lg" placeholder="Min 8 chars, upper + lower + digit" required minLength={8} />
                <p className="text-xs text-gray-400 mt-1">At least 8 characters with an uppercase letter, a lowercase letter, and a digit.</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Role</label>
                <select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value as typeof form.role })}
                  className="w-full px-4 py-2 border border-gray-300 rounded-lg">
                  {ROLES.map(r => <option key={r} value={r}>{r.charAt(0).toUpperCase() + r.slice(1)}</option>)}
                </select>
              </div>
            </div>
            {createError && (
              <div className="px-4 py-2 bg-red-50 border border-red-200 rounded-lg text-sm text-red-700">{createError}</div>
            )}
            <button type="submit" disabled={createUser.isPending}
              className="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
              {createUser.isPending ? 'Creating...' : 'Create User'}
            </button>
          </form>
        </div>
      )}

      {/* Edit error banner */}
      {editError && (
        <div className="mb-4 px-4 py-2 bg-red-50 border border-red-200 rounded-lg text-sm text-red-700">{editError}</div>
      )}

      {/* Users Table */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 overflow-hidden">
        <table className="w-full">
          <thead className="bg-gray-50 border-b border-gray-100">
            <tr>
              <th className="text-left px-6 py-3 text-xs font-medium text-gray-500 uppercase">Name</th>
              <th className="text-left px-6 py-3 text-xs font-medium text-gray-500 uppercase">Email</th>
              <th className="text-left px-6 py-3 text-xs font-medium text-gray-500 uppercase">Role</th>
              <th className="text-left px-6 py-3 text-xs font-medium text-gray-500 uppercase">Created</th>
              <th className="text-right px-6 py-3 text-xs font-medium text-gray-500 uppercase">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-50">
            {isLoading && <tr><td colSpan={5} className="px-6 py-8 text-center text-gray-400">Loading...</td></tr>}
            {users.map((user) => (
              <tr key={user.id} className="hover:bg-gray-50">
                <td className="px-6 py-4">
                  {editingId === user.id ? (
                    <input value={editForm.name} onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
                      className="px-2 py-1 border rounded text-sm w-full" />
                  ) : (
                    <span className="font-medium text-sm">{user.name}</span>
                  )}
                </td>
                <td className="px-6 py-4">
                  {editingId === user.id ? (
                    <input value={editForm.email} onChange={(e) => setEditForm({ ...editForm, email: e.target.value })}
                      className="px-2 py-1 border rounded text-sm w-full" />
                  ) : (
                    <span className="text-sm text-gray-600">{user.email}</span>
                  )}
                </td>
                <td className="px-6 py-4">
                  {editingId === user.id ? (
                    <select value={editForm.role} onChange={(e) => setEditForm({ ...editForm, role: e.target.value })}
                      className="px-2 py-1 border rounded text-sm">
                      {ROLES.map(r => <option key={r} value={r}>{r.charAt(0).toUpperCase() + r.slice(1)}</option>)}
                    </select>
                  ) : (
                    <RoleBadge role={user.role} />
                  )}
                </td>
                <td className="px-6 py-4 text-sm text-gray-500">
                  {new Date(user.created_at).toLocaleDateString()}
                </td>
                <td className="px-6 py-4 text-right">
                  <div className="flex items-center justify-end gap-2">
                    {editingId === user.id ? (
                      <>
                        <button onClick={handleUpdate}
                          className="px-3 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-700">
                          Save
                        </button>
                        <button onClick={() => { setEditingId(null); setEditError('') }}
                          className="px-3 py-1 text-xs bg-gray-200 text-gray-700 rounded hover:bg-gray-300">
                          Cancel
                        </button>
                      </>
                    ) : (
                      <>
                        <button onClick={() => setManagingServersId(user.id)}
                          className="p-1.5 text-gray-400 hover:text-blue-600" title="Manage Servers">
                          <Server size={16} />
                        </button>
                        <button onClick={() => startEdit(user)}
                          className="p-1.5 text-gray-400 hover:text-blue-600" title="Edit">
                          <Edit2 size={16} />
                        </button>
                        <button onClick={() => setChangingPasswordId(user.id)}
                          className="p-1.5 text-gray-400 hover:text-amber-600" title="Change Password">
                          <KeyRound size={16} />
                        </button>
                        <button onClick={() => { if (confirm(`Delete user ${user.name}?`)) deleteUser.mutate(user.id) }}
                          className="p-1.5 text-gray-400 hover:text-red-600" title="Delete">
                          <Trash2 size={16} />
                        </button>
                      </>
                    )}
                  </div>
                </td>
              </tr>
            ))}
            {!isLoading && users.length === 0 && (
              <tr><td colSpan={5} className="px-6 py-8 text-center text-gray-400">No users yet.</td></tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Server Assignment Modal */}
      {managingServersId && (
        <ServerAssignmentModal
          userId={managingServersId}
          onClose={() => setManagingServersId(null)}
        />
      )}

      {/* Change Password Modal */}
      {changingPasswordId && (
        <ChangePasswordModal
          userId={changingPasswordId}
          userName={users.find(u => u.id === changingPasswordId)?.name ?? ''}
          onClose={() => setChangingPasswordId(null)}
        />
      )}
    </div>
  )
}

function ChangePasswordModal({ userId, userName, onClose }: { userId: number; userName: string; onClose: () => void }) {
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [showNew, setShowNew] = useState(false)
  const [showConfirm, setShowConfirm] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)
  const updateUser = useUpdateUser()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (newPassword.length < 8) {
      setError('Mật khẩu phải có ít nhất 8 ký tự')
      return
    }
    if (newPassword !== confirmPassword) {
      setError('Mật khẩu xác nhận không khớp')
      return
    }
    try {
      await updateUser.mutateAsync({ id: userId, password: newPassword } as never)
      setSuccess(true)
      setTimeout(onClose, 1500)
    } catch {
      setError('Đổi mật khẩu thất bại. Vui lòng thử lại.')
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-md mx-4">
        <div className="flex items-center justify-between p-6 border-b">
          <div className="flex items-center gap-2">
            <KeyRound size={18} className="text-amber-600" />
            <h2 className="text-lg font-semibold">Đổi mật khẩu</h2>
          </div>
          <button onClick={onClose} className="p-1 text-gray-400 hover:text-gray-600"><X size={20} /></button>
        </div>

        <form onSubmit={handleSubmit} className="p-6 space-y-4">
          <p className="text-sm text-gray-500">
            Đổi mật khẩu cho <span className="font-medium text-gray-800">{userName}</span>
          </p>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Mật khẩu mới</label>
            <div className="relative">
              <input
                type={showNew ? 'text' : 'password'}
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                className="w-full px-4 py-2 pr-10 border border-gray-300 rounded-lg focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
                placeholder="Tối thiểu 8 ký tự"
                autoFocus
              />
              <button type="button" onClick={() => setShowNew(v => !v)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600">
                {showNew ? <EyeOff size={16} /> : <Eye size={16} />}
              </button>
            </div>
            {newPassword && newPassword.length < 8 && (
              <p className="mt-1 text-xs text-red-500">Cần ít nhất 8 ký tự</p>
            )}
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Xác nhận mật khẩu</label>
            <div className="relative">
              <input
                type={showConfirm ? 'text' : 'password'}
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                className="w-full px-4 py-2 pr-10 border border-gray-300 rounded-lg focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
                placeholder="Nhập lại mật khẩu mới"
              />
              <button type="button" onClick={() => setShowConfirm(v => !v)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600">
                {showConfirm ? <EyeOff size={16} /> : <Eye size={16} />}
              </button>
            </div>
            {confirmPassword && newPassword !== confirmPassword && (
              <p className="mt-1 text-xs text-red-500">Mật khẩu không khớp</p>
            )}
          </div>

          {error && (
            <div className="px-3 py-2 bg-red-50 text-red-700 text-sm rounded-lg">✗ {error}</div>
          )}
          {success && (
            <div className="px-3 py-2 bg-green-50 text-green-700 text-sm rounded-lg">✓ Đổi mật khẩu thành công!</div>
          )}

          <div className="flex gap-3 pt-2">
            <button type="submit"
              disabled={updateUser.isPending || success}
              className="flex-1 px-4 py-2 bg-amber-600 text-white font-medium rounded-lg hover:bg-amber-700 disabled:opacity-50 transition-colors">
              {updateUser.isPending ? 'Đang lưu...' : 'Đổi mật khẩu'}
            </button>
            <button type="button" onClick={onClose}
              className="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200">
              Hủy
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function ServerAssignmentModal({ userId, onClose }: { userId: number; onClose: () => void }) {
  const { data: assignedServers, isLoading } = useUserServers(userId)
  const { data: allServersData } = useServers()
  const assignServer = useAssignServer()
  const unassignServer = useUnassignServer()

  const allServers = allServersData?.data || []
  const assignedIds = new Set((assignedServers || []).map(s => s.id))
  const unassignedServers = allServers.filter(s => !assignedIds.has(s.id))

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[80vh] overflow-hidden flex flex-col">
        <div className="flex items-center justify-between p-6 border-b">
          <h2 className="text-lg font-semibold">Manage Server Access</h2>
          <button onClick={onClose} className="p-1 text-gray-400 hover:text-gray-600"><X size={20} /></button>
        </div>

        <div className="p-6 overflow-y-auto flex-1">
          {/* Assigned Servers */}
          <h3 className="text-sm font-medium text-gray-700 mb-3">Assigned Servers</h3>
          {isLoading ? (
            <p className="text-sm text-gray-400 mb-4">Loading...</p>
          ) : (assignedServers || []).length === 0 ? (
            <p className="text-sm text-gray-400 mb-4">No servers assigned yet.</p>
          ) : (
            <div className="space-y-2 mb-6">
              {(assignedServers || []).map(server => (
                <div key={server.id} className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                  <div>
                    <p className="text-sm font-medium">{server.name}</p>
                    <p className="text-xs text-gray-500 font-mono">{server.ip_address}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <StatusBadge status={server.status} />
                    <button
                      onClick={() => unassignServer.mutate({ userId, serverId: server.id })}
                      className="p-1 text-gray-400 hover:text-red-600" title="Remove">
                      <X size={16} />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}

          {/* Available Servers */}
          {unassignedServers.length > 0 && (
            <>
              <h3 className="text-sm font-medium text-gray-700 mb-3">Available Servers</h3>
              <div className="space-y-2">
                {unassignedServers.map(server => (
                  <div key={server.id} className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                    <div>
                      <p className="text-sm font-medium">{server.name}</p>
                      <p className="text-xs text-gray-500 font-mono">{server.ip_address}</p>
                    </div>
                    <button
                      onClick={() => assignServer.mutate({ userId, server_id: server.id })}
                      className="px-3 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-700">
                      Assign
                    </button>
                  </div>
                ))}
              </div>
            </>
          )}
        </div>

        <div className="p-4 border-t">
          <button onClick={onClose} className="w-full px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200">
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
