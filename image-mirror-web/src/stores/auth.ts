import { create } from "zustand"

import { api } from "@/api/client"
import type { TokenPair, User } from "@/types"

type PersistedAuth = {
  user: User
  tokens: TokenPair
}

type AuthState = {
  user: User | null
  tokens: TokenPair | null
  hydrated: boolean
  setSession: (user: User, tokens: TokenPair) => void
  logout: () => void
  hydrate: () => void
  refreshMe: () => Promise<void>
}

const storageKey = "image-mirror-auth"

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  tokens: null,
  hydrated: false,
  setSession: (user, tokens) => {
    localStorage.setItem(storageKey, JSON.stringify({ user, tokens }))
    set({ user, tokens, hydrated: true })
  },
  logout: () => {
    localStorage.removeItem(storageKey)
    set({ user: null, tokens: null, hydrated: true })
  },
  hydrate: () => {
    const raw = localStorage.getItem(storageKey)
    if (!raw) {
      set({ hydrated: true })
      return
    }
    try {
      const parsed = JSON.parse(raw) as PersistedAuth
      set({ user: parsed.user, tokens: parsed.tokens, hydrated: true })
    } catch {
      localStorage.removeItem(storageKey)
      set({ hydrated: true })
    }
  },
  refreshMe: async () => {
    if (!get().tokens?.accessToken) return
    const { data } = await api.get<{ user: User }>("/api/users/me")
    const tokens = get().tokens
    if (tokens) {
      localStorage.setItem(storageKey, JSON.stringify({ user: data.user, tokens }))
      set({ user: data.user })
    }
  },
}))
