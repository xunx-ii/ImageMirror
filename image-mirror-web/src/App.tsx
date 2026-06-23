import { useEffect } from "react"
import { Navigate, Route, Routes } from "react-router-dom"

import { AppLayout } from "@/components/app-layout"
import { AdminRoute, ProtectedRoute } from "@/components/protected-route"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/sonner"
import { AdminPage } from "@/pages/admin-page"
import { ApiKeysPage } from "@/pages/api-keys-page"
import { AuthPage } from "@/pages/auth-page"
import { BillingPage } from "@/pages/billing-page"
import { DashboardPage } from "@/pages/dashboard-page"
import { DocsPage } from "@/pages/docs-page"
import { GalleryPage } from "@/pages/gallery-page"
import { GeneratePage } from "@/pages/generate-page"
import { useAuthStore } from "@/stores/auth"

function App() {
  const hydrate = useAuthStore((state) => state.hydrate)

  useEffect(() => {
    hydrate()
  }, [hydrate])

  return (
    <TooltipProvider>
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
      <Toaster richColors closeButton />
    </TooltipProvider>
  )
}

export default App
