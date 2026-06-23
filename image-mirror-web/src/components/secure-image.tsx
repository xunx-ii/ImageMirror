import { useEffect, useRef, useState } from "react"
import type { ImgHTMLAttributes } from "react"

import { api } from "@/api/client"
import { Skeleton } from "@/components/ui/skeleton"

type SecureImageProps = Omit<ImgHTMLAttributes<HTMLImageElement>, "alt" | "src"> & {
  imageId: string
  alt: string
}

export function SecureImage({ imageId, alt, className, style, loading = "lazy", decoding = "async", ...props }: SecureImageProps) {
  const placeholderRef = useRef<HTMLDivElement | null>(null)
  const [visibleState, setVisibleState] = useState({ imageId: "", visible: false })
  const [state, setState] = useState<{
    imageId: string
    url: string | null
    failed: boolean
  }>({ imageId, url: null, failed: false })
  const shouldLoad = loading === "eager" || (visibleState.imageId === imageId && visibleState.visible)

  useEffect(() => {
    if (shouldLoad) return
    const node = placeholderRef.current
    if (!node || !("IntersectionObserver" in window)) {
      window.requestAnimationFrame(() => setVisibleState({ imageId, visible: true }))
      return
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setVisibleState({ imageId, visible: true })
          observer.disconnect()
        }
      },
      { rootMargin: "240px" }
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [imageId, shouldLoad])

  useEffect(() => {
    if (!shouldLoad) return
    let revoked: string | null = null
    let cancelled = false

    api
      .get<Blob>(`/api/images/${imageId}/file`, { responseType: "blob" })
      .then((response) => {
        if (cancelled) return
        revoked = URL.createObjectURL(response.data)
        setState({ imageId, url: revoked, failed: false })
      })
      .catch(() => {
        if (!cancelled) setState({ imageId, url: null, failed: true })
      })

    return () => {
      cancelled = true
      if (revoked) URL.revokeObjectURL(revoked)
    }
  }, [imageId, shouldLoad])

  if (!shouldLoad || state.imageId !== imageId) {
    return <Skeleton ref={placeholderRef} className="aspect-square w-full rounded-lg" />
  }

  if (state.failed) {
    return (
      <div className="flex aspect-square w-full items-center justify-center rounded-lg bg-muted text-sm text-muted-foreground">
        图片不可用
      </div>
    )
  }

  if (!state.url) {
    return <Skeleton className="aspect-square w-full rounded-lg" />
  }

  return <img src={state.url} alt={alt} className={className} style={style} loading={loading} decoding={decoding} {...props} />
}
