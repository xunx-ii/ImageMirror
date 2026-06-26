import { useCallback, useEffect, useMemo, useState } from "react"
import { Check, Download, Eye, GalleryHorizontalEnd, RefreshCw, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { ImageViewerDialog } from "@/components/image-viewer-dialog"
import { PageHeader } from "@/components/page-header"
import { SecureImage } from "@/components/secure-image"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import { downloadImage, downloadSelectedImages, expiresIn, formatDate } from "@/lib/format"
import type { ImageGeneration } from "@/types"

export function GalleryPage() {
  const [images, setImages] = useState<ImageGeneration[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [selectionMode, setSelectionMode] = useState(false)
  const [detail, setDetail] = useState<ImageGeneration | null>(null)
  const [viewer, setViewer] = useState<ImageGeneration | null>(null)
  const [pendingDeleteIds, setPendingDeleteIds] = useState<string[]>([])
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
      setDetail((state) => (state && ids.includes(state.id) ? null : state))
      setViewer((state) => (state && ids.includes(state.id) ? null : state))
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setBusy(false)
    }
  }

  function requestDeleteImages(ids: string[]) {
    const uniqueIds = Array.from(new Set(ids))
    if (uniqueIds.length === 0) return
    setPendingDeleteIds(uniqueIds)
  }

  async function confirmDeleteImages() {
    const ids = pendingDeleteIds
    setPendingDeleteIds([])
    await deleteImages(ids)
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

  function exitSelectionMode() {
    setSelectionMode(false)
    setSelected([])
  }

  function openImage(image: ImageGeneration) {
    if (selectionMode) {
      toggleSelected(image.id)
      return
    }
    if (image.status === "COMPLETED") {
      setViewer(image)
      return
    }
    setDetail(image)
  }

  return (
    <>
      <PageHeader
        title="我的图片"
        action={
          <div className="flex flex-wrap gap-2">
            {images.length > 0 && !selectionMode && (
              <Button variant="outline" onClick={() => setSelectionMode(true)}>
                <Check data-icon="inline-start" />
                多选
              </Button>
            )}
            {selectionMode && images.length > 0 && (
              <Button variant="outline" onClick={toggleAll}>
                <Check data-icon="inline-start" />
                {allSelected ? "取消全选" : "全选"}
              </Button>
            )}
            {selectionMode && (
              <>
                <Button variant="outline" disabled={busy || completedSelected.length === 0} onClick={() => void downloadSelected()}>
                  <Download data-icon="inline-start" />
                  下载 {completedSelected.length}
                </Button>
                <Button variant="outline" disabled={busy || selected.length === 0} onClick={() => requestDeleteImages(selected)}>
                  <Trash2 data-icon="inline-start" />
                  删除 {selected.length}
                </Button>
                <Button variant="outline" onClick={exitSelectionMode}>
                  退出多选
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
              <Card key={image.id} className={cn(selectionMode && isSelected && "ring-2 ring-ring")}>
                <CardContent className="flex flex-col gap-3 p-3">
                  <button type="button" className="relative text-left" onClick={() => openImage(image)}>
                    {image.status === "COMPLETED" ? (
                      <SecureImage imageId={image.id} alt={image.prompt} maxEdge={256} className="aspect-square rounded-lg object-cover" />
                    ) : (
                      <div className="flex aspect-square items-center justify-center rounded-lg bg-muted text-sm text-muted-foreground">
                        {image.status}
                      </div>
                    )}
                    {selectionMode && (
                      <span className={cn("absolute right-2 top-2 flex size-7 items-center justify-center rounded-md border bg-background", isSelected ? "opacity-100" : "opacity-70")}>
                        {isSelected && <Check />}
                      </span>
                    )}
                  </button>
                  <div className="flex flex-col gap-2">
                    <div className="line-clamp-2 min-h-10 text-sm">{image.prompt}</div>
                    <div className="flex flex-wrap gap-1">
                      <Badge variant="secondary">{image.status}</Badge>
                      <Badge variant="outline">{image.creditsCost} credits</Badge>
                    </div>
                    <div className="text-xs text-muted-foreground">创建 {formatDate(image.createdAt)}</div>
                    <div className="text-xs text-muted-foreground">过期 {expiresIn(image.expiresAt)}</div>
                    <div className="grid grid-cols-3 gap-2">
                      <Button variant="outline" onClick={() => setDetail(image)}>
                        <Eye data-icon="inline-start" />
                        详情
                      </Button>
                      <Button variant="outline" disabled={image.status !== "COMPLETED"} onClick={() => void downloadImage(image.id)}>
                        <Download data-icon="inline-start" />
                        下载
                      </Button>
                      <Button variant="outline" disabled={busy} onClick={() => requestDeleteImages([image.id])}>
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

      <Dialog open={!!detail} onOpenChange={(open) => !open && setDetail(null)}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>图片详情</DialogTitle>
          </DialogHeader>
          {detail && (
            <div className="flex max-h-[70vh] flex-col gap-4 overflow-auto">
              {detail.status === "COMPLETED" && (
                <SecureImage imageId={detail.id} alt={detail.prompt} maxEdge={768} className="max-h-[420px] w-full rounded-lg object-contain" />
              )}
              <div className="grid gap-3 text-sm sm:grid-cols-2">
                <DetailItem label="状态" value={detail.status} />
                <DetailItem label="积分" value={`${detail.creditsCost} credits`} />
                <DetailItem label="模型" value={detail.model} />
                <DetailItem label="尺寸" value={detail.size} />
                <DetailItem label="质量" value={detail.quality} />
                <DetailItem label="参考图" value={`${detail.referenceCount}`} />
                <DetailItem label="创建时间" value={formatDate(detail.createdAt)} />
                <DetailItem label="更新时间" value={formatDate(detail.updatedAt)} />
                <DetailItem label="过期时间" value={formatDate(detail.expiresAt)} />
                <DetailItem label="任务 ID" value={detail.id} />
              </div>
              <div className="flex flex-col gap-2 rounded-lg border p-3 text-sm">
                <div className="font-medium">提示词</div>
                <div className="whitespace-pre-wrap text-muted-foreground">{detail.prompt}</div>
              </div>
              {detail.status === "FAILED" && (
                <div className="flex flex-col gap-2 rounded-lg border p-3 text-sm">
                  <div className="font-medium">失败原因</div>
                  <div className="whitespace-pre-wrap text-muted-foreground">{detail.errorMessage || "暂无失败原因"}</div>
                </div>
              )}
              <div className="flex flex-wrap gap-2">
                <Button variant="outline" disabled={detail.status !== "COMPLETED"} onClick={() => void downloadImage(detail.id)}>
                  <Download data-icon="inline-start" />
                  下载
                </Button>
                <Button variant="outline" disabled={busy} onClick={() => requestDeleteImages([detail.id])}>
                  <Trash2 data-icon="inline-start" />
                  删除
                </Button>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={pendingDeleteIds.length > 0} onOpenChange={(open) => !open && setPendingDeleteIds([])}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认删除图片</DialogTitle>
          </DialogHeader>
          <div className="text-sm text-muted-foreground">
            确认删除 {pendingDeleteIds.length} 张图片？删除后不可恢复。
          </div>
          <DialogFooter>
            <Button variant="outline" disabled={busy} onClick={() => setPendingDeleteIds([])}>
              取消
            </Button>
            <Button variant="destructive" disabled={busy} onClick={() => void confirmDeleteImages()}>
              <Trash2 data-icon="inline-start" />
              删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ImageViewerDialog image={viewer} open={!!viewer} onOpenChange={(open) => !open && setViewer(null)} />
    </>
  )
}

function DetailItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-1 rounded-lg border p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="truncate font-medium">{value}</div>
    </div>
  )
}
