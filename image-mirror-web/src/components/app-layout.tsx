import {
  BadgeCent,
  GalleryHorizontalEnd,
  ImagePlus,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Shield,
} from "lucide-react"
import { NavLink, Outlet, useNavigate } from "react-router-dom"

import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { useAuthStore } from "@/stores/auth"

const links = [
  { to: "/dashboard", label: "概览", icon: LayoutDashboard },
  { to: "/generate", label: "生成", icon: ImagePlus },
  { to: "/gallery", label: "图库", icon: GalleryHorizontalEnd },
  { to: "/billing", label: "账单", icon: BadgeCent },
  { to: "/api-keys", label: "API Key", icon: KeyRound },
]

export function AppLayout() {
  const navigate = useNavigate()
  const user = useAuthStore((state) => state.user)
  const logout = useAuthStore((state) => state.logout)

  return (
    <div className="min-h-svh bg-background text-foreground">
      <div className="grid min-h-svh lg:grid-cols-[240px_1fr]">
        <aside className="border-b bg-muted/30 lg:border-r lg:border-b-0">
          <div className="flex h-full flex-col gap-4 p-4">
            <div className="flex items-center gap-3 px-1">
              <div className="flex size-9 items-center justify-center rounded-lg bg-primary text-primary-foreground">
                <ImagePlus />
              </div>
              <div className="min-w-0">
                <div className="font-heading text-base font-semibold">ImageMirror</div>
                <div className="truncate text-xs text-muted-foreground">OpenAI 图像中转平台</div>
              </div>
            </div>
            <Separator />
            <nav className="flex flex-col gap-1">
              {links.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) =>
                    [
                      "flex h-9 items-center gap-2 rounded-lg px-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
                      isActive ? "bg-background text-foreground shadow-sm ring-1 ring-border" : "",
                    ].join(" ")
                  }
                >
                  <item.icon data-icon="inline-start" />
                  {item.label}
                </NavLink>
              ))}
              {user?.role === "ADMIN" && (
                <NavLink
                  to="/admin"
                  className={({ isActive }) =>
                    [
                      "flex h-9 items-center gap-2 rounded-lg px-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
                      isActive ? "bg-background text-foreground shadow-sm ring-1 ring-border" : "",
                    ].join(" ")
                  }
                >
                  <Shield data-icon="inline-start" />
                  管理
                </NavLink>
              )}
            </nav>
            <div className="mt-auto flex flex-col gap-3 rounded-lg border bg-background p-3">
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">{user?.email}</div>
                <div className="text-xs text-muted-foreground">余额 {user?.balance ?? 0} credits</div>
              </div>
              <Button
                variant="outline"
                onClick={() => {
                  logout()
                  navigate("/login")
                }}
              >
                <LogOut data-icon="inline-start" />
                退出
              </Button>
            </div>
          </div>
        </aside>
        <main className="min-w-0">
          <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 p-4 md:p-6">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
