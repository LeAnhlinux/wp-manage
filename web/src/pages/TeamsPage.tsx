import { useState } from 'react'
import { Plus, Users, Server, Trash2, UserPlus, Shield, Crown, X } from 'lucide-react'
import { useTeams, useCreateTeam, useDeleteTeam, useUpdateTeam, useAddTeamMember, useRemoveTeamMember, useUpdateTeamMember, useAssignTeamServer, useUnassignTeamServer, useMigrateTeams } from '@/hooks/useTeams'
import { usePermissions } from '@/hooks/usePermissions'
import type { Team, User as UserType, Server as ServerType } from '@/types'
import api from '@/lib/api'
import { useQuery } from '@tanstack/react-query'

export default function TeamsPage() {
  const { isAdmin, can } = usePermissions()
  const { data: teamsData, isLoading } = useTeams(1, 100)
  const createTeam = useCreateTeam()
  const deleteTeam = useDeleteTeam()
  const migrateTeams = useMigrateTeams()

  const [showCreate, setShowCreate] = useState(false)
  const [selectedTeam, setSelectedTeam] = useState<Team | null>(null)
  const [newName, setNewName] = useState('')
  const [newDesc, setNewDesc] = useState('')

  const teams = (teamsData as unknown as { data: Team[] })?.data ?? []

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    await createTeam.mutateAsync({ name: newName, description: newDesc })
    setNewName('')
    setNewDesc('')
    setShowCreate(false)
  }

  if (isLoading) return <div className="p-6">Loading teams...</div>

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Teams</h1>
          <p className="text-sm text-gray-500 mt-1">Manage teams and server access</p>
        </div>
        <div className="flex gap-2">
          {isAdmin && (
            <button
              onClick={() => migrateTeams.mutate()}
              disabled={migrateTeams.isPending}
              className="px-3 py-2 text-sm bg-gray-100 hover:bg-gray-200 rounded-lg"
            >
              {migrateTeams.isPending ? 'Migrating...' : 'Migrate from UserServer'}
            </button>
          )}
          {(isAdmin || can('teams', 'create')) && (
            <button
              onClick={() => setShowCreate(true)}
              className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
            >
              <Plus size={16} /> Create Team
            </button>
          )}
        </div>
      </div>

      {/* Create Team Form */}
      {showCreate && (
        <div className="bg-white rounded-xl border p-6">
          <h2 className="text-lg font-semibold mb-4">Create Team</h2>
          <form onSubmit={handleCreate} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Team Name</label>
              <input
                type="text"
                value={newName}
                onChange={e => setNewName(e.target.value)}
                className="w-full px-3 py-2 border rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                placeholder="e.g., Engineering Team"
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
              <input
                type="text"
                value={newDesc}
                onChange={e => setNewDesc(e.target.value)}
                className="w-full px-3 py-2 border rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                placeholder="Optional description"
              />
            </div>
            <div className="flex gap-2">
              <button type="submit" disabled={createTeam.isPending} className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {createTeam.isPending ? 'Creating...' : 'Create'}
              </button>
              <button type="button" onClick={() => setShowCreate(false)} className="px-4 py-2 bg-gray-100 rounded-lg hover:bg-gray-200">
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Teams Grid */}
      {teams.length === 0 ? (
        <div className="text-center py-12 bg-white rounded-xl border">
          <Users size={48} className="mx-auto text-gray-300 mb-4" />
          <h3 className="text-lg font-medium text-gray-900">No teams yet</h3>
          <p className="text-sm text-gray-500 mt-1">Create a team to organize users and server access</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {teams.map((team: Team) => (
            <div
              key={team.id}
              className={`bg-white rounded-xl border p-5 cursor-pointer transition-all hover:shadow-md ${selectedTeam?.id === team.id ? 'ring-2 ring-blue-500' : ''}`}
              onClick={() => setSelectedTeam(selectedTeam?.id === team.id ? null : team)}
            >
              <div className="flex items-start justify-between">
                <div>
                  <h3 className="font-semibold text-gray-900">{team.name}</h3>
                  {team.description && <p className="text-sm text-gray-500 mt-1">{team.description}</p>}
                </div>
                {isAdmin && (
                  <button
                    onClick={e => { e.stopPropagation(); deleteTeam.mutate(team.id) }}
                    className="p-1.5 text-gray-400 hover:text-red-500 hover:bg-red-50 rounded"
                  >
                    <Trash2 size={14} />
                  </button>
                )}
              </div>
              <div className="mt-4 flex items-center gap-4 text-sm text-gray-500">
                <span className="flex items-center gap-1"><Users size={14} /> {team.members?.length || 0} members</span>
                <span className="flex items-center gap-1"><Server size={14} /> {team.servers?.length || 0} servers</span>
              </div>
              {team.owner && (
                <div className="mt-2 text-xs text-gray-400">Owner: {team.owner.name}</div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Team Detail Panel */}
      {selectedTeam && <TeamDetailPanel team={selectedTeam} onClose={() => setSelectedTeam(null)} />}
    </div>
  )
}

function TeamDetailPanel({ team, onClose }: { team: Team; onClose: () => void }) {
  const { isAdmin } = usePermissions()
  const updateTeam = useUpdateTeam()
  const addMember = useAddTeamMember()
  const removeMember = useRemoveTeamMember()
  const updateMember = useUpdateTeamMember()
  const assignServer = useAssignTeamServer()
  const unassignServer = useUnassignTeamServer()

  const [showAddMember, setShowAddMember] = useState(false)
  const [showAddServer, setShowAddServer] = useState(false)
  const [selectedUserId, setSelectedUserId] = useState('')
  const [memberRole, setMemberRole] = useState<'admin' | 'member'>('member')
  const [selectedServerId, setSelectedServerId] = useState('')
  const [editName, setEditName] = useState(team.name)
  const [editDesc, setEditDesc] = useState(team.description)
  const [editing, setEditing] = useState(false)

  // Fetch all users for add member dropdown
  const { data: usersData } = useQuery({
    queryKey: ['all-users'],
    queryFn: async () => {
      const { data } = await api.get('/users?per_page=100')
      return data
    },
    enabled: showAddMember,
  })
  const allUsers: UserType[] = (usersData as { data?: UserType[] })?.data ?? []

  // Fetch all servers for add server dropdown
  const { data: serversData } = useQuery({
    queryKey: ['all-servers'],
    queryFn: async () => {
      const { data } = await api.get('/servers?per_page=100')
      return data
    },
    enabled: showAddServer,
  })
  const allServers: ServerType[] = (serversData as { data?: ServerType[] })?.data ?? []

  const handleSave = async () => {
    await updateTeam.mutateAsync({ teamId: team.id, name: editName, description: editDesc })
    setEditing(false)
  }

  const handleAddMember = async (e: React.FormEvent) => {
    e.preventDefault()
    await addMember.mutateAsync({ teamId: team.id, user_id: parseInt(selectedUserId), role: memberRole })
    setShowAddMember(false)
    setSelectedUserId('')
    setMemberRole('member')
  }

  const handleAddServer = async (e: React.FormEvent) => {
    e.preventDefault()
    await assignServer.mutateAsync({ teamId: team.id, server_id: parseInt(selectedServerId) })
    setShowAddServer(false)
    setSelectedServerId('')
  }

  return (
    <div className="bg-white rounded-xl border p-6 space-y-6">
      <div className="flex items-center justify-between">
        {editing ? (
          <div className="flex-1 space-y-2">
            <input value={editName} onChange={e => setEditName(e.target.value)} className="w-full px-3 py-2 border rounded-lg text-lg font-semibold" />
            <input value={editDesc} onChange={e => setEditDesc(e.target.value)} className="w-full px-3 py-2 border rounded-lg text-sm" placeholder="Description" />
            <div className="flex gap-2">
              <button onClick={handleSave} className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded-lg">Save</button>
              <button onClick={() => setEditing(false)} className="px-3 py-1.5 bg-gray-100 text-sm rounded-lg">Cancel</button>
            </div>
          </div>
        ) : (
          <div>
            <h2 className="text-xl font-bold text-gray-900">{team.name}</h2>
            {team.description && <p className="text-sm text-gray-500">{team.description}</p>}
            {isAdmin && <button onClick={() => setEditing(true)} className="text-xs text-blue-600 hover:underline mt-1">Edit</button>}
          </div>
        )}
        <button onClick={onClose} className="p-2 text-gray-400 hover:text-gray-600 hover:bg-gray-100 rounded-lg">
          <X size={18} />
        </button>
      </div>

      {/* Members Section */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h3 className="font-semibold text-gray-900 flex items-center gap-2"><Users size={16} /> Members ({team.members?.length || 0})</h3>
          {isAdmin && (
            <button onClick={() => setShowAddMember(!showAddMember)} className="flex items-center gap-1 text-sm text-blue-600 hover:text-blue-800">
              <UserPlus size={14} /> Add
            </button>
          )}
        </div>

        {showAddMember && (
          <form onSubmit={handleAddMember} className="flex gap-2 mb-3">
            <select value={selectedUserId} onChange={e => setSelectedUserId(e.target.value)} className="flex-1 px-3 py-2 border rounded-lg text-sm" required>
              <option value="">Select user...</option>
              {allUsers.filter(u => !team.members?.some(m => m.user_id === u.id)).map(u => (
                <option key={u.id} value={u.id}>{u.name} ({u.email})</option>
              ))}
            </select>
            <select value={memberRole} onChange={e => setMemberRole(e.target.value as 'admin' | 'member')} className="px-3 py-2 border rounded-lg text-sm">
              <option value="member">Member</option>
              <option value="admin">Admin</option>
            </select>
            <button type="submit" disabled={addMember.isPending} className="px-3 py-2 bg-blue-600 text-white text-sm rounded-lg">Add</button>
          </form>
        )}

        <div className="space-y-2">
          {team.members?.map(member => (
            <div key={member.id} className="flex items-center justify-between py-2 px-3 bg-gray-50 rounded-lg">
              <div className="flex items-center gap-3">
                <div className="w-8 h-8 rounded-full bg-blue-100 text-blue-700 flex items-center justify-center text-sm font-medium">
                  {member.user?.name?.[0]?.toUpperCase() || '?'}
                </div>
                <div>
                  <div className="text-sm font-medium text-gray-900 flex items-center gap-1">
                    {member.user?.name}
                    {member.user_id === team.owner_id && <Crown size={12} className="text-yellow-500" />}
                  </div>
                  <div className="text-xs text-gray-500">{member.user?.email}</div>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {isAdmin && member.user_id !== team.owner_id && (
                  <select
                    value={member.role}
                    onChange={e => updateMember.mutate({ teamId: team.id, userId: member.user_id, role: e.target.value as 'admin' | 'member' })}
                    className="text-xs px-2 py-1 border rounded"
                  >
                    <option value="member">Member</option>
                    <option value="admin">Admin</option>
                  </select>
                )}
                <span className={`text-xs px-2 py-0.5 rounded-full ${member.role === 'admin' ? 'bg-purple-100 text-purple-700' : 'bg-gray-100 text-gray-600'}`}>
                  {member.role === 'admin' ? <Shield size={10} className="inline mr-1" /> : null}
                  {member.role}
                </span>
                {isAdmin && member.user_id !== team.owner_id && (
                  <button
                    onClick={() => removeMember.mutate({ teamId: team.id, userId: member.user_id })}
                    className="p-1 text-gray-400 hover:text-red-500"
                  >
                    <Trash2 size={14} />
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Servers Section */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h3 className="font-semibold text-gray-900 flex items-center gap-2"><Server size={16} /> Servers ({team.servers?.length || 0})</h3>
          {isAdmin && (
            <button onClick={() => setShowAddServer(!showAddServer)} className="flex items-center gap-1 text-sm text-blue-600 hover:text-blue-800">
              <Plus size={14} /> Assign
            </button>
          )}
        </div>

        {showAddServer && (
          <form onSubmit={handleAddServer} className="flex gap-2 mb-3">
            <select value={selectedServerId} onChange={e => setSelectedServerId(e.target.value)} className="flex-1 px-3 py-2 border rounded-lg text-sm" required>
              <option value="">Select server...</option>
              {allServers.filter(s => !team.servers?.some(ts => ts.server_id === s.id)).map(s => (
                <option key={s.id} value={s.id}>{s.name} ({s.ip_address})</option>
              ))}
            </select>
            <button type="submit" disabled={assignServer.isPending} className="px-3 py-2 bg-blue-600 text-white text-sm rounded-lg">Assign</button>
          </form>
        )}

        <div className="space-y-2">
          {team.servers?.map(ts => (
            <div key={ts.id} className="flex items-center justify-between py-2 px-3 bg-gray-50 rounded-lg">
              <div className="flex items-center gap-3">
                <Server size={16} className="text-gray-400" />
                <div>
                  <div className="text-sm font-medium text-gray-900">{ts.server?.name}</div>
                  <div className="text-xs text-gray-500">{ts.server?.ip_address}</div>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <span className={`text-xs px-2 py-0.5 rounded-full ${ts.server?.status === 'active' ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-600'}`}>
                  {ts.server?.status}
                </span>
                {isAdmin && (
                  <button
                    onClick={() => unassignServer.mutate({ teamId: team.id, serverId: ts.server_id })}
                    className="p-1 text-gray-400 hover:text-red-500"
                  >
                    <Trash2 size={14} />
                  </button>
                )}
              </div>
            </div>
          ))}
          {(!team.servers || team.servers.length === 0) && (
            <p className="text-sm text-gray-400 text-center py-4">No servers assigned</p>
          )}
        </div>
      </div>
    </div>
  )
}
