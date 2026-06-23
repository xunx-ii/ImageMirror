import { lazy, Suspense, useEffect } from "react"
import { Navigate, Route, Routes } from "react-router-dom"

import { AppLayout } from "@/components/app-layout"
import { AdminRoute, ProtectedRoute } from "@/components/protected-route"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/sonner"
import { AuthPage } from "@/pages/auth-page"
import { Skeleton } from "@/components/ui/skeleton"
import { useAuthStore } from "@/stores/auth"

const AdminPage = lazy(() => import("@/pages/admin-page").then((module) => ({ default: module.AdminPage })))
const ApiKeysPage = lazy(() => import("@/pages/api-keys-page").then((module) => ({ default: module.ApiKeysPage })))
const BillingPage = lazy(() => import("@/pages/billing-page").then((module) => ({ default: module.BillingPage })))
const DashboardPage = lazy(() => import("@/pages/dashboard-page").then((module) => ({ default: module.DashboardPage })))
const DocsPage = lazy(() => import("@/pages/docs-page").then((module) => ({ default: module.DocsPage })))
const GalleryPage = lazy(() => import("@/pages/gallery-page").then((module) => ({ default: module.GalleryPage })))
const GeneratePage = lazy(() => import("@/pages/generate-page").then((module) => ({ default: module.GeneratePage })))

function App() {
  const hydrate = useAuthStore((state) => state.hydrate)

  useEffect(() => {
    hydrate()
  }, [hydrate])

  return (
    <TooltipProvider>
      <Suspense fallback={<RouteFallback />}>
        <Routes>
          <Route path="/login" element={<AuthPage mode="login" />} />
          <Route path="/register" element={<AuthPage mode="register" />} />
          <Route element={<ProtectedRoute />}>
            <Route element={<AppLayout />}>
              <Route index element={<Navigate to="/dashboard" replace />} />
              <Route path="/dashboard" element={<DashboardPage />} />
              <Route path="/generate" element={<GeneratePage />} />
              <Route path="/gallery" element={<GalleryPage />} />
              <Route path="/billing" element={<BillingPage />} />
              <Route path="/docs" element={<DocsPage />} />
              <Route path="/api-keys" element={<ApiKeysPage />} />
              <Route element={<AdminRoute />}>
                <Route path="/admin" element={<AdminPage />} />
              </Route>
            </Route>
          </Route>
          <Route path="*" element={<Navigate to="/dashboard" replace />} />
        </Routes>
      </Suspense>
      <Toaster richColors closeButton />
    </TooltipProvider>
  )
}

function RouteFallback() {
  return (
    <div className="min-h-svh bg-background p-4 md:p-6">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-4">
        <Skeleton className="h-10 w-48 rounded-lg" />
        <Skeleton className="h-[420px] rounded-lg" />
      </div>
    </div>
  )
}

export default App
