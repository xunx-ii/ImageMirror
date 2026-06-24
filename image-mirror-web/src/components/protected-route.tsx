import { Navigate, Outlet } from "react-router-dom"

import { useAuthStore } from "@/stores/auth"

export function ProtectedRoute() {
  const user = useAuthStore((state) => state.user)
  const hydrated = useAuthStore((state) => state.hydrated)

  if (!hydrated) return null
  if (!user) return <Navigate to="/login" replace />
  return <Outlet />
}

export function AdminRoute() {
  const user = useAuthStore((state) => state.user)
  if (!user || user.role !== "ADMIN") return <Navigate to="/generate" replace />
  return <Outlet />
}
