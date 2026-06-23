import { useState } from "react"
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
  const [scaleState, setScaleState] = useState({ imageKey: "", value: 1 })
  const scale = scaleState.imageKey === imageKey ? scaleState.value : 1
  const canView = image?.status === "COMPLETED"

  function zoom(delta: number) {
    setScaleState({ imageKey, value: Math.min(4, Math.max(0.25, Number((scale + delta).toFixed(2)))) })
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
                <Button variant="outline" size="icon" aria-label="重置缩放" onClick={() => setScaleState({ imageKey, value: 1 })}>
                  <RotateCcw />
                </Button>
                <Button variant="outline" disabled={!canView} onClick={() => image && void downloadImage(image.id)}>
                  <Download data-icon="inline-start" />
                  下载
                </Button>
              </div>
            </div>
            <div className="min-h-[360px] overflow-auto rounded-lg border bg-muted/30 p-3">
              {canView ? (
                <div className="flex min-h-[60vh] items-center justify-center">
                  <SecureImage
                    imageId={image.id}
                    alt={image.prompt}
                    className={cn("rounded-md object-contain", scale <= 1 && "max-h-[70vh] max-w-full")}
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
