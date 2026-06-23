export const resolutionBuckets = ["1k", "2k", "4k"] as const
const minImagePixels = 655360
const maxImageEdge = 3840
const maxImagePixels = 3840 * 2160

export type ResolutionBucket = (typeof resolutionBuckets)[number]

export function sizeString(width: number, height: number) {
  return `${width}x${height}`
}

export function resolutionBucket(width: number, height: number): ResolutionBucket {
  const longest = Math.max(width, height)
  if (longest <= 1024) return "1k"
  if (longest <= 2048) return "2k"
  return "4k"
}

export function validateImageSize(width: number, height: number) {
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    return "宽高必须大于 0"
  }
  if (width % 16 !== 0 || height % 16 !== 0) {
    return "宽高必须是 16 的倍数"
  }
  if (width > maxImageEdge || height > maxImageEdge) {
    return "最长边不能超过 3840"
  }
  if (width * height < minImagePixels) {
    return "总像素不能低于 655,360"
  }
  if (width * height > maxImagePixels) {
    return "总像素不能超过 3840x2160"
  }
  if (width * 3 < height || height * 3 < width) {
    return "宽高比例必须在 1:3 到 3:1 之间"
  }
  return null
}
