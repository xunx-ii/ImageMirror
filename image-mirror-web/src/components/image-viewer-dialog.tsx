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
  const scaleRef = useRef(1)
  const imageKeyRef = useRef("")
  const dragRef = useRef<{
    pointerId: number
    x: number
    y: number
    scrollLeft: number
    scrollTop: number
  } | null>(null)
  const [scaleState, setScaleState] = useState({ imageKey: "", value: 1 })
  const [dragging, setDragging] = useState(false)
  const scale = scaleState.imageKey === imageKey ? scaleState.value : 1
  const canView = image?.status === "COMPLETED"

  useEffect(() => {
    scaleRef.current = scale
    imageKeyRef.current = imageKey
  }, [imageKey, scale])

  const zoomAt = useCallback((delta: number, origin?: { clientX: number; clientY: number }) => {
    const node = viewportRef.current
    const previous = scaleRef.current
    const next = Math.min(4, Math.max(0.25, Number((previous + delta).toFixed(2))))
    if (next === previous) return

    if (origin && node) {
      const rect = node.getBoundingClientRect()
      const offsetX = origin.clientX - rect.left
      const offsetY = origin.clientY - rect.top
      const scrollLeft = node.scrollLeft
      const scrollTop = node.scrollTop
      const ratio = next / previous

      setScaleState({ imageKey: imageKeyRef.current, value: next })
      window.requestAnimationFrame(() => {
        node.scrollLeft = (scrollLeft + offsetX) * ratio - offsetX
        node.scrollTop = (scrollTop + offsetY) * ratio - offsetY
      })
      return
    }

    setScaleState({ imageKey: imageKeyRef.current, value: next })
  }, [])

  useEffect(() => {
    const node = viewportRef.current
    if (!node || !canView) return

    function handleNativeWheel(event: WheelEvent) {
      event.preventDefault()
      event.stopPropagation()
      zoomAt(event.deltaY < 0 ? 0.1 : -0.1, event)
    }

    node.addEventListener("wheel", handleNativeWheel, { passive: false })
    return () => node.removeEventListener("wheel", handleNativeWheel)
  }, [canView, zoomAt])

  function zoom(delta: number) {
    const node = viewportRef.current
    const rect = node?.getBoundingClientRect()
    zoomAt(delta, rect ? { clientX: rect.left + rect.width / 2, clientY: rect.top + rect.height / 2 } : undefined)
  }

  function resetScale() {
    setScaleState({ imageKey, value: 1 })
    window.requestAnimationFrame(() => {
      viewportRef.current?.scrollTo({ left: 0, top: 0 })
    })
  }

  function startDrag(event: ReactPointerEvent<HTMLDivElement>) {
    if (!canView || event.button !== 0) return
    const node = viewportRef.current
    if (!node) return

    event.preventDefault()
    node.setPointerCapture(event.pointerId)
    dragRef.current = {
      pointerId: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      scrollLeft: node.scrollLeft,
      scrollTop: node.scrollTop,
    }
    setDragging(true)
  }

  function moveDrag(event: ReactPointerEvent<HTMLDivElement>) {
    const node = viewportRef.current
    const drag = dragRef.current
    if (!node || !drag || drag.pointerId !== event.pointerId) return

    event.preventDefault()
    node.scrollLeft = drag.scrollLeft - (event.clientX - drag.x)
    node.scrollTop = drag.scrollTop - (event.clientY - drag.y)
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
    if (!nextOpen) setScaleState({ imageKey: "", value: 1 })
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-[96vw] sm:max-w-5xl">
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
                "min-h-[360px] overflow-auto rounded-lg border bg-muted/30 p-3 select-none",
                canView && (dragging ? "cursor-grabbing" : "cursor-grab")
              )}
              onPointerDown={startDrag}
              onPointerMove={moveDrag}
              onPointerUp={stopDrag}
              onPointerCancel={stopDrag}
            >
              {canView ? (
                <div className="flex min-h-[60vh] items-center justify-center">
                  <SecureImage
                    imageId={image.id}
                    alt={image.prompt}
                    className={cn("rounded-md object-contain", scale <= 1 && "max-h-[70vh] max-w-full")}
                    draggable={false}
                    style={scale > 1 ? { width: `${Math.round(scale * 100)}%`, maxWidth: "none" } : { width: `${Math.round(scale * 100)}%` }}
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
