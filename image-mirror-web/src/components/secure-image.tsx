import { useEffect, useState } from "react"
import type { CSSProperties } from "react"

import { api } from "@/api/client"
import { Skeleton } from "@/components/ui/skeleton"

type SecureImageProps = {
  imageId: string
  alt: string
  className?: string
  style?: CSSProperties
}

export function SecureImage({ imageId, alt, className, style }: SecureImageProps) {
  const [state, setState] = useState<{
    imageId: string
    url: string | null
    failed: boolean
  }>({ imageId, url: null, failed: false })

  useEffect(() => {
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
  }, [imageId])

  if (state.imageId !== imageId) {
    return <Skeleton className="aspect-square w-full rounded-lg" />
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

  return <img src={state.url} alt={alt} className={className} style={style} />
}
