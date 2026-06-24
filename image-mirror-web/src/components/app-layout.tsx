import {
  BadgeCent,
  Bell,
  FileText,
  GalleryHorizontalEnd,
  Gift,
  ImagePlus,
  KeyRound,
  LogOut,
  Shield,
} from "lucide-react"
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom"
import { useEffect, useRef, useState } from "react"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Separator } from "@/components/ui/separator"
import { checkinSettingsUpdatedEvent } from "@/lib/checkin"
import { siteContentUpdatedEvent } from "@/lib/content"
import { renderMarkdown } from "@/lib/markdown"
import { defaultPlatformSettings, mergePlatformSettings, platformDocumentTitle, platformSettingsUpdatedEvent } from "@/lib/platform"
import { useAuthStore } from "@/stores/auth"
import type { CheckinResult, CheckinSettings, CheckinStatus, PlatformSettings, SiteContent } from "@/types"

const links = [
  { to: "/generate", label: "生成", icon: ImagePlus },
  { to: "/gallery", label: "图库", icon: GalleryHorizontalEnd },
  { to: "/billing", label: "账单", icon: BadgeCent },
  { to: "/docs", label: "文档", icon: FileText },
  { to: "/api-keys", label: "API Key", icon: KeyRound },
]

export function AppLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const pathRef = useRef(location.pathname)
  const user = useAuthStore((state) => state.user)
  const tokens = useAuthStore((state) => state.tokens)
  const logout = useAuthStore((state) => state.logout)
  const setSession = useAuthStore((state) => state.setSession)
  const refreshMe = useAuthStore((state) => state.refreshMe)
  const [announcement, setAnnouncement] = useState<SiteContent | null>(null)
  const [announcementOpen, setAnnouncementOpen] = useState(false)
  const [docsVisible, setDocsVisible] = useState(false)
  const [platform, setPlatform] = useState<PlatformSettings>(defaultPlatformSettings)
  const [checkinStatus, setCheckinStatus] = useState<CheckinStatus | null>(null)
  const [checkinLoading, setCheckinLoading] = useState(false)

  useEffect(() => {
    pathRef.current = location.pathname
  }, [location.pathname])

  useEffect(() => {
    let active = true
    const applySettings = (settings: Partial<PlatformSettings> | null | undefined) => {
      const nextSettings = mergePlatformSettings(settings)
      setPlatform(nextSettings)
      document.title = platformDocumentTitle(nextSettings)
    }
    const handleSettingsUpdated = (event: Event) => {
      applySettings((event as CustomEvent<PlatformSettings>).detail)
    }

    document.title = platformDocumentTitle(defaultPlatformSettings)
    window.addEventListener(platformSettingsUpdatedEvent, handleSettingsUpdated)
    api
      .get<PlatformSettings>("/api/settings/platform")
      .then((response) => {
        if (!active) return
        applySettings(response.data)
      })
      .catch(() => {
        document.title = platformDocumentTitle(defaultPlatformSettings)
      })
    return () => {
      active = false
      window.removeEventListener(platformSettingsUpdatedEvent, handleSettingsUpdated)
    }
  }, [])

  useEffect(() => {
    let active = true
    const handleContentUpdated = (event: Event) => {
      const content = (event as CustomEvent<SiteContent>).detail
      if (content.key !== "announcement") return
      setAnnouncement(content)
      if (!content.isActive || !content.body.trim()) {
        setAnnouncementOpen(false)
      }
    }

    window.addEventListener(siteContentUpdatedEvent, handleContentUpdated)
    api
      .get<SiteContent>("/api/content/announcement")
      .then((response) => {
        if (!active) return
        setAnnouncement(response.data)
        if (!response.data.body.trim() || !user?.id || !tokens?.accessToken) return
        const sessionKey = `image-mirror-announcement:${user.id}:${tokens.accessToken.slice(-16)}`
        if (sessionStorage.getItem(sessionKey)) return
        sessionStorage.setItem(sessionKey, "1")
        setAnnouncementOpen(true)
      })
      .catch((error) => {
        const status = (error as { response?: { status?: number } }).response?.status
        if (active && status === 404) {
          setAnnouncement(null)
          setAnnouncementOpen(false)
        }
        if (status !== 404) toast.error(errorMessage(error))
      })
    return () => {
      active = false
      window.removeEventListener(siteContentUpdatedEvent, handleContentUpdated)
    }
  }, [tokens?.accessToken, user?.id])

  useEffect(() => {
    let active = true
    const handleContentUpdated = (event: Event) => {
      const content = (event as CustomEvent<SiteContent>).detail
      if (content.key !== "docs") return
      setDocsVisible(content.isActive)
      if (!content.isActive && pathRef.current === "/docs") {
        navigate("/generate", { replace: true })
      }
    }

    window.addEventListener(siteContentUpdatedEvent, handleContentUpdated)
    api
      .get<SiteContent>("/api/content/docs")
      .then(() => {
        if (active) setDocsVisible(true)
      })
      .catch((error) => {
        const status = (error as { response?: { status?: number } }).response?.status
        if (active) setDocsVisible(false)
        if (status === 404 && pathRef.current === "/docs") {
          navigate("/generate", { replace: true })
        }
      })
    return () => {
      active = false
      window.removeEventListener(siteContentUpdatedEvent, handleContentUpdated)
    }
  }, [navigate])

  useEffect(() => {
    let active = true
    const applySettings = (settings: CheckinSettings) => {
      setCheckinStatus((state) => ({
        enabled: settings.enabled,
        credits: settings.credits,
        checkedIn: state?.checkedIn ?? false,
        lastCheckin: state?.lastCheckin,
      }))
    }
    const handleSettingsUpdated = (event: Event) => {
      applySettings((event as CustomEvent<CheckinSettings>).detail)
    }

    window.addEventListener(checkinSettingsUpdatedEvent, handleSettingsUpdated)
    api
      .get<CheckinStatus>("/api/checkin/status")
      .then((response) => {
        if (active) setCheckinStatus(response.data)
      })
      .catch((error) => {
        const status = (error as { response?: { status?: number } }).response?.status
        if (status !== 404) toast.error(errorMessage(error))
      })
    return () => {
      active = false
      window.removeEventListener(checkinSettingsUpdatedEvent, handleSettingsUpdated)
    }
  }, [tokens?.accessToken, user?.id])

  async function checkin() {
    if (!tokens) return
    setCheckinLoading(true)
    try {
      const { data } = await api.post<CheckinResult>("/api/checkin")
      setCheckinStatus(data.status)
      setSession(data.user, tokens)
      toast.success(`签到成功，获得 ${data.status.credits} credits`)
      await refreshMe()
    } catch (error) {
      toast.error(errorMessage(error))
      void api
        .get<CheckinStatus>("/api/checkin/status")
        .then((response) => setCheckinStatus(response.data))
        .catch(() => undefined)
    } finally {
      setCheckinLoading(false)
    }
  }

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
                <div className="font-heading text-base font-semibold">{platform.siteTitle}</div>
                <div className="truncate text-xs text-muted-foreground">{platform.siteSubtitle}</div>
              </div>
            </div>
            <Separator />
            <nav className="flex flex-col gap-1">
              {links.filter((item) => item.to !== "/docs" || docsVisible).map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) =>
                    [
                      "flex h-9 items-center gap-2 rounded-lg px-2 text-sm font-medium text-muted-foreground transition-all duration-150 hover:-translate-y-px hover:bg-muted hover:text-foreground active:translate-y-px",
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
                      "flex h-9 items-center gap-2 rounded-lg px-2 text-sm font-medium text-muted-foreground transition-all duration-150 hover:-translate-y-px hover:bg-muted hover:text-foreground active:translate-y-px",
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
            <div className="flex justify-end gap-2">
              {checkinStatus?.enabled && (
                <Button variant="outline" onClick={() => void checkin()} disabled={checkinLoading || checkinStatus.checkedIn}>
                  <Gift data-icon="inline-start" />
                  {checkinStatus.checkedIn ? "已签到" : `每日签到 +${checkinStatus.credits}`}
                </Button>
              )}
              <Button variant="outline" size="icon" aria-label="公告" onClick={() => setAnnouncementOpen(true)} disabled={!announcement?.isActive || !announcement.body.trim()}>
                <Bell />
              </Button>
            </div>
            <div key={location.pathname} className="animate-in fade-in-0 slide-in-from-bottom-2 duration-200">
              <Outlet />
            </div>
          </div>
        </main>
      </div>
      <Dialog open={announcementOpen} onOpenChange={setAnnouncementOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{announcement?.title || "公告"}</DialogTitle>
          </DialogHeader>
          <div className="flex max-h-[60vh] flex-col gap-3 overflow-auto text-sm">
            {announcement?.body ? renderMarkdown(announcement.body) : "暂无公告"}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
