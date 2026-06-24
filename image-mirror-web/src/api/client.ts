import axios from "axios"

const authStorageKey = "image-mirror-auth"
export const unauthorizedEventName = "image-mirror:unauthorized"
let unauthorizedDispatched = false

export const api = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL ?? "",
  timeout: 310_000,
})

api.interceptors.request.use((config) => {
  const raw = localStorage.getItem(authStorageKey)
  if (raw) {
    try {
      const parsed = JSON.parse(raw) as { tokens?: { accessToken?: string } }
      if (parsed.tokens?.accessToken) {
        unauthorizedDispatched = false
        config.headers.Authorization = `Bearer ${parsed.tokens.accessToken}`
      }
    } catch {
      localStorage.removeItem(authStorageKey)
    }
  }
  return config
})

api.interceptors.response.use(
  (response) => {
    unauthorizedDispatched = false
    return response
  },
  (error: unknown) => {
    if (axios.isAxiosError(error) && error.response?.status === 401) {
      localStorage.removeItem(authStorageKey)
      if (!unauthorizedDispatched && typeof window !== "undefined") {
        unauthorizedDispatched = true
        window.dispatchEvent(new CustomEvent(unauthorizedEventName))
      }
    }
    return Promise.reject(error)
  }
)

export function errorMessage(error: unknown) {
  if (axios.isAxiosError(error)) {
    const message = error.response?.data?.error?.message
    if (typeof message === "string") return message
    if (error.message) return error.message
  }
  if (error instanceof Error) return error.message
  return "操作失败"
}
