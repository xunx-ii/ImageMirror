import axios from "axios"

export const api = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL ?? "",
  timeout: 310_000,
})

api.interceptors.request.use((config) => {
  const raw = localStorage.getItem("image-mirror-auth")
  if (raw) {
    try {
      const parsed = JSON.parse(raw) as { tokens?: { accessToken?: string } }
      if (parsed.tokens?.accessToken) {
        config.headers.Authorization = `Bearer ${parsed.tokens.accessToken}`
      }
    } catch {
      localStorage.removeItem("image-mirror-auth")
    }
  }
  return config
})

export function errorMessage(error: unknown) {
  if (axios.isAxiosError(error)) {
    const message = error.response?.data?.error?.message
    if (typeof message === "string") return message
    if (error.message) return error.message
  }
  if (error instanceof Error) return error.message
  return "操作失败"
}
