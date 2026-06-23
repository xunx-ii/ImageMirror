import { useCallback, useEffect, useRef, useState } from "react"
import type { PointerEvent as ReactPointerEvent } from "react"
import { Download, RotateCcw, ZoomIn, ZoomOut } from "lucide-react"

import { SecureImage } from "@/components/secure-image"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { downloadImage, formatDate } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { ImageGeneration } from "@/types"

type ImageViewerDialogProps = {
  image: ImageGeneration | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ImageViewerDialog({ image, open, onOpenChange }: ImageViewerDialogProps) {
  const imageKey = open && image ? image.id : ""
  const viewportRef = useRef<HTMLDivElement | null>(null)
  const imageKeyRef = useRef("")
  const dragRef = useRef<{
    pointerId: number
    x: number
    y: number
    translateX: number
    translateY: number
  } | null>(null)
  const viewRef = useRef({ scale: 1, x: 0, y: 0 })
  const [viewState, setViewState] = useState({ imageKey: "", scale: 1, x: 0, y: 0 })
  const [dragging, setDragging] = useState(false)
  const activeView = viewState.imageKey === imageKey ? viewState : { imageKey, scale: 1, x: 0, y: 0 }
  const scale = activeView.scale
  const canView = image?.status === "COMPLETED"

  useEffect(() => {
    viewRef.current = { scale: activeView.scale, x: activeView.x, y: activeView.y }
    imageKeyRef.current = imageKey
  }, [activeView.scale, activeView.x, activeView.y, imageKey])

  const zoomAt = useCallback((delta: number, origin?: { clientX: number; clientY: number }) => {
    const node = viewportRef.current
    const previous = viewRef.current
    const nextScale = Math.min(4, Math.max(0.25, Number((previous.scale + delta).toFixed(2))))
    if (nextScale === previous.scale) return

    if (!origin || !node || nextScale <= 1) {
      setViewState({ imageKey: imageKeyRef.current, scale: nextScale, x: 0, y: 0 })
      return
    }

    const rect = node.getBoundingClientRect()
    const originX = origin.clientX - rect.left - rect.width / 2
    const originY = origin.clientY - rect.top - rect.height / 2
    const ratio = nextScale / previous.scale

    setViewState({
      imageKey: imageKeyRef.current,
      scale: nextScale,
      x: previous.x * ratio + originX * (1 - ratio),
      y: previous.y * ratio + originY * (1 - ratio),
    })
  }, [])

  useEffect(() => {
    const node = viewportRef.current
    if (!node || !canView) return

    function handleNativeWheel(event: WheelEvent) {
      event.preventDefault()
      event.stopPropagation()
      zoomAt(event.deltaY < 0 ? 0.1 : -0.1, event)
    }

    node.addEventListener("wheel", handleNativeWheel, { passive: false, capture: true })
    return () => node.removeEventListener("wheel", handleNativeWheel, { capture: true })
  }, [canView, zoomAt])

  function zoom(delta: number) {
    const node = viewportRef.current
    const rect = node?.getBoundingClientRect()
    zoomAt(delta, rect ? { clientX: rect.left + rect.width / 2, clientY: rect.top + rect.height / 2 } : undefined)
  }

  function resetScale() {
    setViewState({ imageKey, scale: 1, x: 0, y: 0 })
  }

  function startDrag(event: ReactPointerEvent<HTMLDivElement>) {
    if (!canView || event.button !== 0 || viewRef.current.scale <= 1) return
    const node = viewportRef.current
    if (!node) return

    event.preventDefault()
    node.setPointerCapture(event.pointerId)
    dragRef.current = {
      pointerId: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      translateX: viewRef.current.x,
      translateY: viewRef.current.y,
    }
    setDragging(true)
  }

  function moveDrag(event: ReactPointerEvent<HTMLDivElement>) {
    const node = viewportRef.current
    const drag = dragRef.current
    if (!node || !drag || drag.pointerId !== event.pointerId) return

    event.preventDefault()
    setViewState({
      imageKey: imageKeyRef.current,
      scale: viewRef.current.scale,
      x: drag.translateX + event.clientX - drag.x,
      y: drag.translateY + event.clientY - drag.y,
    })
  }

  function stopDrag(event: ReactPointerEvent<HTMLDivElement>) {
    const node = viewportRef.current
    const drag = dragRef.current
    if (node && drag?.pointerId === event.pointerId && node.hasPointerCapture(event.pointerId)) {
      node.releasePointerCapture(event.pointerId)
    }
    dragRef.current = null
    setDragging(false)
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      dragRef.current = null
      setDragging(false)
      setViewState({ imageKey: "", scale: 1, x: 0, y: 0 })
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-h-[96svh] max-w-[96vw] overflow-hidden sm:max-w-5xl">
        <DialogHeader>
          <DialogTitle>查看图片</DialogTitle>
        </DialogHeader>
        {image && (
          <div className="flex max-h-[82vh] flex-col gap-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="min-w-0 text-sm text-muted-foreground">
                <span className="mr-3">{image.size}</span>
                <span>{formatDate(image.createdAt)}</span>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="outline" size="icon" aria-label="缩小" onClick={() => zoom(-0.25)}>
                  <ZoomOut />
                </Button>
                <div className="w-16 text-center text-sm tabular-nums">{Math.round(scale * 100)}%</div>
                <Button variant="outline" size="icon" aria-label="放大" onClick={() => zoom(0.25)}>
                  <ZoomIn />
                </Button>
                <Button variant="outline" size="icon" aria-label="重置缩放" onClick={resetScale}>
                  <RotateCcw />
                </Button>
                <Button variant="outline" disabled={!canView} onClick={() => image && void downloadImage(image.id)}>
                  <Download data-icon="inline-start" />
                  下载
                </Button>
              </div>
            </div>
            <div
              ref={viewportRef}
              className={cn(
                "h-[min(68svh,720px)] min-h-[360px] overflow-hidden rounded-lg border bg-muted/30 p-3 select-none touch-none overscroll-none",
                canView && scale > 1 && (dragging ? "cursor-grabbing" : "cursor-grab")
              )}
              onPointerDown={startDrag}
              onPointerMove={moveDrag}
              onPointerUp={stopDrag}
              onPointerCancel={stopDrag}
            >
              {canView ? (
                <div className="flex h-full items-center justify-center overflow-hidden">
                  <SecureImage
                    imageId={image.id}
                    alt={image.prompt}
                    className="max-h-[70vh] max-w-full rounded-md object-contain will-change-transform"
                    draggable={false}
                    loading="eager"
                    style={{
                      transform: `translate3d(${activeView.x}px, ${activeView.y}px, 0) scale(${scale})`,
                      transformOrigin: "center center",
                    }}
                  />
                </div>
              ) : (
                <div className="flex min-h-[360px] items-center justify-center text-sm text-muted-foreground">
                  图片未生成完成
                </div>
              )}
            </div>
            <div className="line-clamp-2 text-sm text-muted-foreground">{image.prompt}</div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
