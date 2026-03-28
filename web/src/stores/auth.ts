import { create } from 'zustand'
import api from '@/lib/api'
import type { User } from '@/types'

// H15 mitigation: use sessionStorage instead of localStorage for JWT token.
// sessionStorage is tab-scoped (cleared on tab close) and does NOT persist across
// browser sessions — reduces the XSS exposure window vs localStorage.
// Full mitigation requires httpOnly cookies (future work).
const tokenStorage = sessionStorage

interface AuthState {
  user: User | null
  token: string | null
  loading: boolean
  login: (email: string, password: string) => Promise<{ requires_2fa?: boolean; email?: string }>
  loginWith2FA: (email: string, password: string, code: string) => Promise<void>
  register: (email: string, password: string, name: string) => Promise<void>
  logout: () => void
  loadUser: () => Promise<void>
}

export const useAuth = create<AuthState>((set) => ({
  user: null,
  token: tokenStorage.getItem('token'),
  loading: false,

  login: async (email, password) => {
    const { data } = await api.post('/auth/login', { email, password })
    if (data.data.requires_2fa) {
      return { requires_2fa: true, email: data.data.email }
    }
    const token = data.data.token
    tokenStorage.setItem('token', token)
    set({ token, user: data.data.user })
    return {}
  },

  loginWith2FA: async (email, password, code) => {
    const { data } = await api.post('/auth/2fa/validate', { email, password, code })
    const token = data.data.token
    tokenStorage.setItem('token', token)
    set({ token, user: data.data.user })
  },

  register: async (email, password, name) => {
    await api.post('/auth/register', { email, password, name })
  },

  logout: () => {
    tokenStorage.removeItem('token')
    set({ user: null, token: null })
  },

  loadUser: async () => {
    try {
      set({ loading: true })
      const { data } = await api.get('/auth/me')
      set({ user: data.data, loading: false })
    } catch {
      tokenStorage.removeItem('token')
      set({ user: null, token: null, loading: false })
    }
  },
}))
