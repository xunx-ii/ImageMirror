import { useCallback, useEffect, useMemo, useState } from "react"
import { BadgeCent, GalleryHorizontalEnd, ImagePlus, KeyRound, RefreshCw } from "lucide-react"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { ImageViewerDialog } from "@/components/image-viewer-dialog"
import { PageHeader } from "@/components/page-header"
import { SecureImage } from "@/components/secure-image"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import { formatDate } from "@/lib/format"
import { useAuthStore } from "@/stores/auth"
import type { ApiKey, ImageGeneration } from "@/types"

export function DashboardPage() {
  const user = useAuthStore((state) => state.user)
  const refreshMe = useAuthStore((state) => state.refreshMe)
  const [images, setImages] = useState<ImageGeneration[]>([])
  const [keys, setKeys] = useState<ApiKey[]>([])
  const [viewer, setViewer] = useState<ImageGeneration | null>(null)
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [imagesResponse, keysResponse] = await Promise.all([
        api.get<{ data: ImageGeneration[] }>("/api/images?limit=6"),
        api.get<{ data: ApiKey[] }>("/api/api-keys"),
        refreshMe(),
      ])
      setImages(imagesResponse.data.data ?? [])
      setKeys(keysResponse.data.data ?? [])
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setLoading(false)
    }
  }, [refreshMe])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void load()
    }, 0)
    return () => window.clearTimeout(timer)
  }, [load])

  const completed = useMemo(() => images.filter((image) => image.status === "COMPLETED"), [images])

  return (
    <>
      <PageHeader
        title="概览"
        action={
          <Button variant="outline" onClick={load}>
            <RefreshCw data-icon="inline-start" />
            刷新
          </Button>
        }
      />

      <div className="grid gap-4 md:grid-cols-3">
        <MetricCard icon={BadgeCent} label="可用积分" value={user?.balance ?? 0} />
        <MetricCard icon={GalleryHorizontalEnd} label="最近图片" value={images.length} />
        <MetricCard icon={KeyRound} label="活跃 API Key" value={keys.filter((key) => key.status === "ACTIVE").length} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>最近生成</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {Array.from({ length: 3 }).map((_, index) => (
                <Skeleton key={index} className="aspect-square rounded-lg" />
              ))}
            </div>
          ) : completed.length > 0 ? (
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {completed.map((image) => (
                <div key={image.id} className="flex flex-col gap-2 rounded-lg border p-2">
                  <button type="button" className="text-left" onClick={() => setViewer(image)}>
                    <SecureImage imageId={image.id} alt={image.prompt} className="aspect-square rounded-md object-cover" />
                  </button>
                  <div className="flex items-center justify-between gap-2">
                    <div className="truncate text-sm">{image.prompt}</div>
                    <Badge variant="secondary">{formatDate(image.createdAt)}</Badge>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <ImagePlus />
                </EmptyMedia>
                <EmptyTitle>还没有完成的图片</EmptyTitle>
                <EmptyDescription>生成完成后会出现在这里，文件会在 24 小时后自动过期。</EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
        </CardContent>
      </Card>
      <ImageViewerDialog image={viewer} open={!!viewer} onOpenChange={(open) => !open && setViewer(null)} />
    </>
  )
}

function MetricCard({ icon: Icon, label, value }: { icon: typeof BadgeCent; label: string; value: number }) {
  return (
    <Card>
      <CardContent className="flex items-center gap-3 p-4">
        <div className="flex size-10 items-center justify-center rounded-lg bg-muted">
          <Icon />
        </div>
        <div>
          <div className="text-sm text-muted-foreground">{label}</div>
          <div className="text-2xl font-semibold">{value}</div>
        </div>
      </CardContent>
    </Card>
  )
}
