import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import type {
  Team,
  TeamMember,
  TeamServerAssignment,
  CreateTeamRequest,
  UpdateTeamRequest,
  AddTeamMemberRequest,
  UpdateTeamMemberRequest,
  AssignTeamServerRequest,
  APIResponse,
  PaginatedResponse,
} from '@/types'

// List teams
export function useTeams(page = 1, perPage = 20) {
  return useQuery({
    queryKey: ['teams', page, perPage],
    queryFn: async () => {
      const { data } = await api.get<PaginatedResponse<Team>>(`/teams?page=${page}&per_page=${perPage}`)
      return data
    },
  })
}

// Get single team
export function useTeam(teamId: number | undefined) {
  return useQuery({
    queryKey: ['teams', teamId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<Team>>(`/teams/${teamId}`)
      return data.data
    },
    enabled: !!teamId,
  })
}

// Create team
export function useCreateTeam() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: CreateTeamRequest) => {
      const { data } = await api.post<APIResponse<Team>>('/teams', req)
      return data.data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['teams'] }),
  })
}

// Update team
export function useUpdateTeam() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, ...req }: UpdateTeamRequest & { teamId: number }) => {
      const { data } = await api.put<APIResponse<Team>>(`/teams/${teamId}`, req)
      return data.data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['teams'] }),
  })
}

// Delete team
export function useDeleteTeam() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (teamId: number) => {
      await api.delete(`/teams/${teamId}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['teams'] }),
  })
}

// Add member
export function useAddTeamMember() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, ...req }: AddTeamMemberRequest & { teamId: number }) => {
      const { data } = await api.post<APIResponse<TeamMember>>(`/teams/${teamId}/members`, req)
      return data.data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['teams'] }),
  })
}

// Update member role
export function useUpdateTeamMember() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, userId, ...req }: UpdateTeamMemberRequest & { teamId: number; userId: number }) => {
      const { data } = await api.put<APIResponse<TeamMember>>(`/teams/${teamId}/members/${userId}`, req)
      return data.data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['teams'] }),
  })
}

// Remove member
export function useRemoveTeamMember() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, userId }: { teamId: number; userId: number }) => {
      await api.delete(`/teams/${teamId}/members/${userId}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['teams'] }),
  })
}

// Assign server to team
export function useAssignTeamServer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, ...req }: AssignTeamServerRequest & { teamId: number }) => {
      const { data } = await api.post<APIResponse<TeamServerAssignment>>(`/teams/${teamId}/servers`, req)
      return data.data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['teams'] }),
  })
}

// Unassign server from team
export function useUnassignTeamServer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, serverId }: { teamId: number; serverId: number }) => {
      await api.delete(`/teams/${teamId}/servers/${serverId}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['teams'] }),
  })
}

// Migrate from UserServer to TeamServer
export function useMigrateTeams() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIResponse<{ migrated_users: number }>>('/teams/migrate')
      return data.data
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['teams'] }),
  })
}
