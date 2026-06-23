import { useCallback, useEffect, useMemo, useState } from "react"
import { Check, Download, GalleryHorizontalEnd, RefreshCw, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { PageHeader } from "@/components/page-header"
import { SecureImage } from "@/components/secure-image"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import { downloadImage, downloadSelectedImages, expiresIn, formatDate } from "@/lib/format"
import type { ImageGeneration } from "@/types"

export function GalleryPage() {
  const [images, setImages] = useState<ImageGeneration[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)

  const completedSelected = useMemo(
    () => selected.filter((id) => images.some((image) => image.id === id && image.status === "COMPLETED")),
    [images, selected]
  )
  const allSelected = images.length > 0 && selected.length === images.length

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const { data } = await api.get<{ data: ImageGeneration[] }>("/api/images?limit=100")
      const items = data.data ?? []
      setImages(items)
      setSelected((state) => state.filter((id) => items.some((image) => image.id === id)))
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void load()
    }, 0)
    return () => window.clearTimeout(timer)
  }, [load])

  function toggleSelected(id: string) {
    setSelected((state) => (state.includes(id) ? state.filter((item) => item !== id) : [...state, id]))
  }

  function toggleAll() {
    setSelected(allSelected ? [] : images.map((image) => image.id))
  }

  async function deleteImages(ids: string[]) {
    if (ids.length === 0) return
    setBusy(true)
    try {
      await api.post("/api/images/bulk-delete", { ids })
      toast.success(`已删除 ${ids.length} 张图片`)
      setSelected((state) => state.filter((id) => !ids.includes(id)))
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  async function downloadSelected() {
    if (completedSelected.length === 0) return
    setBusy(true)
    try {
      await downloadSelectedImages(completedSelected)
      toast.success("已开始打包下载")
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <PageHeader
        title="我的图片"
        action={
          <div className="flex flex-wrap gap-2">
            {images.length > 0 && (
              <Button variant="outline" onClick={toggleAll}>
                <Check data-icon="inline-start" />
                {allSelected ? "取消全选" : "全选"}
              </Button>
            )}
            {selected.length > 0 && (
              <>
                <Button variant="outline" disabled={busy || completedSelected.length === 0} onClick={() => void downloadSelected()}>
                  <Download data-icon="inline-start" />
                  下载 {completedSelected.length}
                </Button>
                <Button variant="outline" disabled={busy} onClick={() => void deleteImages(selected)}>
                  <Trash2 data-icon="inline-start" />
                  删除 {selected.length}
                </Button>
              </>
            )}
            <Button variant="outline" onClick={load}>
              <RefreshCw data-icon="inline-start" />
              刷新
            </Button>
          </div>
        }
      />
      {loading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 8 }).map((_, index) => (
            <Skeleton key={index} className="aspect-square rounded-lg" />
          ))}
        </div>
      ) : images.length === 0 ? (
        <Empty className="min-h-[420px]">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <GalleryHorizontalEnd />
            </EmptyMedia>
            <EmptyTitle>图库为空</EmptyTitle>
            <EmptyDescription>生成任务提交后，记录会保存在这里。</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {images.map((image) => {
            const isSelected = selected.includes(image.id)
            return (
              <Card key={image.id} className={cn(isSelected && "ring-2 ring-ring")}>
                <CardContent className="flex flex-col gap-3 p-3">
                  <button type="button" className="relative text-left" onClick={() => toggleSelected(image.id)}>
                    {image.status === "COMPLETED" ? (
                      <SecureImage imageId={image.id} alt={image.prompt} className="aspect-square rounded-lg object-cover" />
                    ) : (
                      <div className="flex aspect-square items-center justify-center rounded-lg bg-muted text-sm text-muted-foreground">
                        {image.status}
                      </div>
                    )}
                    <span className={cn("absolute right-2 top-2 flex size-7 items-center justify-center rounded-md border bg-background", isSelected ? "opacity-100" : "opacity-70")}>
                      {isSelected && <Check />}
                    </span>
                  </button>
                  <div className="flex flex-col gap-2">
                    <div className="line-clamp-2 min-h-10 text-sm">{image.prompt}</div>
                    <div className="flex flex-wrap gap-1">
                      <Badge variant="secondary">{image.status}</Badge>
                      <Badge variant="outline">{image.creditsCost} credits</Badge>
                    </div>
                    <div className="text-xs text-muted-foreground">创建 {formatDate(image.createdAt)}</div>
                    <div className="text-xs text-muted-foreground">过期 {expiresIn(image.expiresAt)}</div>
                    <div className="grid grid-cols-2 gap-2">
                      <Button variant="outline" disabled={image.status !== "COMPLETED"} onClick={() => void downloadImage(image.id)}>
                        <Download data-icon="inline-start" />
                        下载
                      </Button>
                      <Button variant="outline" disabled={busy} onClick={() => void deleteImages([image.id])}>
                        <Trash2 data-icon="inline-start" />
                        删除
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}
    </>
  )
}
