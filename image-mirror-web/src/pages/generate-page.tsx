import { useEffect, useMemo, useState } from "react"
import type { FormEvent } from "react"
import { Download, ImagePlus, Loader2, RefreshCw, Sparkles, Upload, X } from "lucide-react"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { PageHeader } from "@/components/page-header"
import { SecureImage } from "@/components/secure-image"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Progress } from "@/components/ui/progress"
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { downloadImage, expiresIn } from "@/lib/format"
import { resolutionBucket, sizeString, validateImageSize } from "@/lib/image-size"
import { useAuthStore } from "@/stores/auth"
import type { ImageGeneration, PlatformSettings, PricingRule } from "@/types"

const qualityOptions = ["low", "medium", "high", "auto"]

function parseDimension(value: string, fallback: number) {
  const next = Number(value)
  if (!Number.isFinite(next)) return fallback
  return Math.max(16, Math.floor(next))
}

export function GeneratePage() {
  const refreshMe = useAuthStore((state) => state.refreshMe)
  const [prompt, setPrompt] = useState("A clean product-style image of a glass teapot on a walnut desk, soft daylight")
  const [width, setWidth] = useState(1024)
  const [height, setHeight] = useState(1024)
  const [quality, setQuality] = useState("medium")
  const [pricing, setPricing] = useState<PricingRule[]>([])
  const [platform, setPlatform] = useState<PlatformSettings>({ maxResolutionBucket: "4k", allow4k: true })
  const [current, setCurrent] = useState<ImageGeneration | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [referenceFiles, setReferenceFiles] = useState<File[]>([])
  const [fileInputKey, setFileInputKey] = useState(0)

  useEffect(() => {
    Promise.all([
      api.get<{ data: PricingRule[] }>("/api/pricing"),
      api.get<PlatformSettings>("/api/settings/platform"),
    ])
      .then(([pricingResponse, platformResponse]) => {
        setPricing(pricingResponse.data.data ?? [])
        setPlatform(platformResponse.data)
      })
      .catch((error) => toast.error(errorMessage(error)))
  }, [])

  const cost = useMemo(() => {
    const bucket = resolutionBucket(width, height)
    return pricing.find((rule) => rule.model === "gpt-image-2" && rule.size === bucket && rule.quality === quality)?.credits
  }, [height, pricing, quality, width])

  const sizeError = useMemo(() => validateImageSize(width, height, platform.maxResolutionBucket), [height, platform.maxResolutionBucket, width])
  const size = sizeString(width, height)
  const bucket = resolutionBucket(width, height)
  const maxEdge = platform.allow4k ? 3840 : 2048
  const maxHeight = platform.allow4k ? 2160 : 2048

  useEffect(() => {
    if (!current || current.status === "COMPLETED" || current.status === "FAILED" || current.status === "EXPIRED") return
    const timer = window.setInterval(async () => {
      try {
        const { data } = await api.get<{ image: ImageGeneration }>(`/api/images/${current.id}/status`)
        setCurrent(data.image)
        if (data.image.status === "COMPLETED") {
          toast.success("图片已生成")
          void refreshMe()
          window.clearInterval(timer)
        }
        if (data.image.status === "FAILED") {
          toast.error(data.image.errorMessage ?? "生成失败")
          void refreshMe()
          window.clearInterval(timer)
        }
      } catch (error) {
        toast.error(errorMessage(error))
      }
    }, 2500)
    return () => window.clearInterval(timer)
  }, [current, refreshMe])

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (sizeError) {
      toast.error(sizeError)
      return
    }
    if (cost == null) {
      toast.error("当前尺寸和质量没有配置定价")
      return
    }
    setSubmitting(true)
    try {
      const form = new FormData()
      form.append("model", "gpt-image-2")
      form.append("prompt", prompt)
      form.append("size", size)
      form.append("quality", quality)
      referenceFiles.forEach((file) => form.append("referenceImages", file))
      const { data } = await api.post<{ image: ImageGeneration }>("/api/images/generate", form)
      setCurrent(data.image)
      setReferenceFiles([])
      setFileInputKey((value) => value + 1)
      toast.success("任务已提交")
      void refreshMe()
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setSubmitting(false)
    }
  }

  function addReferenceFiles(files: FileList | null) {
    if (!files) return
    const accepted = Array.from(files).filter((file) => file.type.startsWith("image/"))
    if (accepted.length !== files.length) {
      toast.error("只能选择图片文件")
    }
    setReferenceFiles((state) => {
      const next = [...state, ...accepted].slice(0, 4)
      if (state.length + accepted.length > 4) {
        toast.error("最多提交 4 张参考图")
      }
      return next
    })
  }

  function removeReferenceFile(index: number) {
    setReferenceFiles((state) => state.filter((_, itemIndex) => itemIndex !== index))
  }

  const busy = current?.status === "PENDING" || current?.status === "PROCESSING"

  return (
    <>
      <PageHeader title="生成工作台" />
      <div className="grid gap-4 lg:grid-cols-[380px_1fr]">
        <Card>
          <CardHeader>
            <CardTitle>参数</CardTitle>
            <CardDescription>生成结果会写入共享图片目录并保留 24 小时。</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="flex flex-col gap-5" onSubmit={submit}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="prompt">提示词</FieldLabel>
                  <Textarea id="prompt" value={prompt} onChange={(event) => setPrompt(event.target.value)} rows={8} required />
                  <FieldDescription>建议包含主体、材质、光线、构图和用途。</FieldDescription>
                </Field>
                <Field>
                  <FieldLabel>尺寸</FieldLabel>
                  <div className="grid grid-cols-2 gap-2">
                    <Input aria-label="宽度" type="number" min={16} max={maxEdge} step={16} value={width} onChange={(event) => setWidth((current) => parseDimension(event.target.value, current))} />
                    <Input aria-label="高度" type="number" min={16} max={maxHeight} step={16} value={height} onChange={(event) => setHeight((current) => parseDimension(event.target.value, current))} />
                  </div>
                  <FieldDescription>{sizeError ?? `当前按 ${bucket.toUpperCase()} 档计费`}</FieldDescription>
                </Field>
                <Field>
                  <FieldLabel>质量</FieldLabel>
                  <Select value={quality} onValueChange={(value) => setQuality(value ?? quality)}>
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        {qualityOptions.map((item) => (
                          <SelectItem key={item} value={item}>
                            {item}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
                <Field>
                  <FieldLabel htmlFor="reference-images">参考图</FieldLabel>
                  <Input
                    key={fileInputKey}
                    id="reference-images"
                    type="file"
                    accept="image/png,image/jpeg,image/webp"
                    multiple
                    onChange={(event) => addReferenceFiles(event.target.files)}
                  />
                  <FieldDescription>可选，最多 4 张 PNG、JPG 或 WEBP。</FieldDescription>
                </Field>
              </FieldGroup>
              {referenceFiles.length > 0 && (
                <div className="flex flex-col gap-2 rounded-lg border bg-muted/20 p-3">
                  <div className="flex items-center gap-2 text-sm font-medium">
                    <Upload />
                    {referenceFiles.length} 张参考图
                  </div>
                  <div className="flex flex-col gap-2">
                    {referenceFiles.map((file, index) => (
                      <div key={`${file.name}-${file.size}-${index}`} className="flex items-center justify-between gap-3 rounded-md bg-background px-3 py-2 text-sm">
                        <span className="truncate">{file.name}</span>
                        <Button type="button" variant="ghost" size="icon" aria-label="移除参考图" onClick={() => removeReferenceFile(index)}>
                          <X />
                        </Button>
                      </div>
                    ))}
                  </div>
                </div>
              )}
              <div className="flex items-center justify-between rounded-lg border bg-muted/30 p-3">
                <span className="text-sm text-muted-foreground">本次预扣</span>
                <Badge variant="secondary">{cost ?? "-"} credits</Badge>
              </div>
              <Button disabled={submitting || busy || !prompt.trim() || !!sizeError || cost == null} type="submit">
                {submitting || busy ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Sparkles data-icon="inline-start" />}
                {submitting ? "提交中" : busy ? "生成中" : "生成图片"}
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div className="flex items-start justify-between gap-3">
              <div>
                <CardTitle>结果</CardTitle>
                <CardDescription>{current ? `状态 ${current.status}` : "提交任务后会在这里显示生成进度。"}</CardDescription>
              </div>
              {current?.status === "COMPLETED" && (
                <Button variant="outline" onClick={() => void downloadImage(current.id)}>
                  <Download data-icon="inline-start" />
                  下载
                </Button>
              )}
            </div>
          </CardHeader>
          <CardContent>
            {!current ? (
              <Empty className="min-h-[420px]">
                <EmptyHeader>
                  <EmptyMedia variant="icon">
                    <ImagePlus />
                  </EmptyMedia>
                  <EmptyTitle>等待生成任务</EmptyTitle>
                  <EmptyDescription>图片完成后可在图库中继续查看，过期前都可以下载。</EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : busy ? (
              <div className="flex min-h-[420px] flex-col justify-center gap-4 rounded-lg border bg-muted/20 p-6">
                <div className="flex items-center gap-2 text-sm font-medium">
                  <RefreshCw className="animate-spin" />
                  Worker 正在处理任务
                </div>
                <Progress value={current.status === "PROCESSING" ? 66 : 30} />
                <div className="text-sm text-muted-foreground">任务 ID {current.id}</div>
              </div>
            ) : current.status === "COMPLETED" ? (
              <div className="flex flex-col gap-3">
                <SecureImage imageId={current.id} alt={current.prompt} className="max-h-[620px] w-full rounded-lg object-contain" />
                <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                  <Badge variant="secondary">{current.size}</Badge>
                  <Badge variant="secondary">{current.quality}</Badge>
                  {current.referenceCount > 0 && <Badge variant="secondary">参考图 {current.referenceCount}</Badge>}
                  <span>过期倒计时 {expiresIn(current.expiresAt)}</span>
                </div>
              </div>
            ) : (
              <div className="rounded-lg border p-4 text-sm text-muted-foreground">{current.errorMessage ?? "任务未完成"}</div>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  )
}
