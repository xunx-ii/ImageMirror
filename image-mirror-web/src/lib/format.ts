import { api } from "@/api/client"

export function formatDate(value?: string) {
  if (!value) return "-"
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value))
}

export function expiresIn(value: string) {
  const ms = new Date(value).getTime() - Date.now()
  if (ms <= 0) return "已过期"
  const hours = Math.floor(ms / 3_600_000)
  const minutes = Math.floor((ms % 3_600_000) / 60_000)
  return `${hours}h ${minutes}m`
}

export async function downloadImage(imageId: string) {
  const response = await api.get<Blob>(`/api/images/${imageId}/file`, {
    responseType: "blob",
  })
  const url = URL.createObjectURL(response.data)
  triggerDownload(url, `${imageId}.png`)
}

export async function downloadSelectedImages(imageIds: string[]) {
  const response = await api.post<Blob>(
    "/api/images/bulk-download",
    { ids: imageIds },
    { responseType: "blob" }
  )
  const url = URL.createObjectURL(response.data)
  triggerDownload(url, "image-mirror-selected.zip")
}

function triggerDownload(url: string, filename: string) {
  const anchor = document.createElement("a")
  anchor.href = url
  anchor.download = filename
  anchor.rel = "noopener"
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
}
