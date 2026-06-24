import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import type { FormEvent } from "react"
import { Download, History, ImagePlus, Loader2, Plus, RefreshCw, SendHorizontal, Upload, X } from "lucide-react"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { ImageViewerDialog } from "@/components/image-viewer-dialog"
import { PageHeader } from "@/components/page-header"
import { SecureImage } from "@/components/secure-image"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Progress } from "@/components/ui/progress"
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { downloadImage, expiresIn, formatDate } from "@/lib/format"
import { resolutionBucket, sizeString, validateImageSize } from "@/lib/image-size"
import { defaultPlatformSettings, mergePlatformSettings, platformSettingsUpdatedEvent } from "@/lib/platform"
import { cn } from "@/lib/utils"
import { useAuthStore } from "@/stores/auth"
import type { ImageGeneration, PlatformSettings, PricingRule } from "@/types"

const qualityOptions = [
  { value: "low", label: "低" },
  { value: "medium", label: "中" },
  { value: "high", label: "高" },
  { value: "auto", label: "自动" },
]

type ReferencePreview = {
  id: string
  file: File
  url: string
}

type GenerateChatIndex = {
  imageToChat: Record<string, string>
  chats: Record<string, string[]>
  updatedAt: Record<string, string>
}

type HistoryThread = {
  id: string
  images: ImageGeneration[]
  latest: ImageGeneration
}

const chatIndexStorageKey = "image-mirror.generate.chat-index"

function createReferenceId(file: File) {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") return crypto.randomUUID()
  return `${file.name}-${file.size}-${file.lastModified}-${Math.random().toString(36).slice(2)}`
}

function qualityLabel(value: string) {
  return qualityOptions.find((item) => item.value === value)?.label ?? value
}

function parseDimensionInput(value: string) {
  const next = Number(value.trim())
  if (!Number.isFinite(next) || next <= 0) return null
  return Math.floor(next)
}

function emptyChatIndex(): GenerateChatIndex {
  return { imageToChat: {}, chats: {}, updatedAt: {} }
}

function readStoredChatIndex(): GenerateChatIndex {
  if (typeof window === "undefined") return emptyChatIndex()
  try {
    const raw = window.localStorage.getItem(chatIndexStorageKey)
    if (!raw) return emptyChatIndex()
    const parsed = JSON.parse(raw) as Partial<GenerateChatIndex>
    return {
      imageToChat: parsed.imageToChat ?? {},
      chats: parsed.chats ?? {},
      updatedAt: parsed.updatedAt ?? {},
    }
  } catch {
    return emptyChatIndex()
  }
}

function writeStoredChatIndex(index: GenerateChatIndex) {
  if (typeof window === "undefined") return
  try {
    window.localStorage.setItem(chatIndexStorageKey, JSON.stringify(index))
  } catch {
    // Ignore quota or privacy-mode failures; the chat still works for this page session.
  }
}

function upsertChatImages(index: GenerateChatIndex, chatId: string, imageIds: string[]) {
  const imageToChat = { ...index.imageToChat }
  const chats = Object.fromEntries(Object.entries(index.chats).map(([id, ids]) => [id, [...ids]]))
  const nextIds = chats[chatId] ? [...chats[chatId]] : []

  imageIds.forEach((imageId) => {
    const previousChatId = imageToChat[imageId]
    if (previousChatId && previousChatId !== chatId) {
      chats[previousChatId] = (chats[previousChatId] ?? []).filter((id) => id !== imageId)
    }
    imageToChat[imageId] = chatId
    if (!nextIds.includes(imageId)) nextIds.push(imageId)
  })

  return {
    imageToChat,
    chats: { ...chats, [chatId]: nextIds },
    updatedAt: { ...index.updatedAt, [chatId]: new Date().toISOString() },
  }
}

function createdAtTime(image: ImageGeneration) {
  return new Date(image.createdAt).getTime()
}

function orderImagesForChat(chatId: string, images: ImageGeneration[], index: GenerateChatIndex) {
  const byId = new Map(images.map((image) => [image.id, image]))
  const storedIds = index.chats[chatId] ?? []
  if (storedIds.length === 0) return [...images].sort((a, b) => createdAtTime(a) - createdAtTime(b))
  const ordered = storedIds.map((id) => byId.get(id)).filter((image): image is ImageGeneration => Boolean(image))
  const orderedIds = new Set(ordered.map((image) => image.id))
  const leftovers = images.filter((image) => !orderedIds.has(image.id)).sort((a, b) => createdAtTime(a) - createdAtTime(b))
  return [...ordered, ...leftovers]
}

function parseImageSize(value: string) {
  const [widthValue, heightValue] = value.split("x")
  const nextWidth = parseDimensionInput(widthValue ?? "")
  const nextHeight = parseDimensionInput(heightValue ?? "")
  if (nextWidth == null || nextHeight == null) return null
  return { width: nextWidth, height: nextHeight }
}

function generationBusy(image?: ImageGeneration | null) {
  return image?.status === "PENDING" || image?.status === "PROCESSING"
}

function statusLabel(value: ImageGeneration["status"]) {
  switch (value) {
    case "PENDING":
      return "排队中"
    case "PROCESSING":
      return "生成中"
    case "COMPLETED":
      return "已完成"
    case "FAILED":
      return "失败"
    case "EXPIRED":
      return "已过期"
    default:
      return value
  }
}

export function GeneratePage() {
  const refreshMe = useAuthStore((state) => state.refreshMe)
  const user = useAuthStore((state) => state.user)
  const referenceUrlRef = useRef<Set<string>>(new Set())
  const messageScrollRef = useRef<HTMLDivElement | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const activeImagesRef = useRef<ImageGeneration[]>([])
  const [prompt, setPrompt] = useState("")
  const [width, setWidth] = useState(1024)
  const [height, setHeight] = useState(1024)
  const [widthInput, setWidthInput] = useState("1024")
  const [heightInput, setHeightInput] = useState("1024")
  const [sizeDraftError, setSizeDraftError] = useState<string | null>(null)
  const [quality, setQuality] = useState("medium")
  const [pricing, setPricing] = useState<PricingRule[]>([])
  const [platform, setPlatform] = useState<PlatformSettings>(defaultPlatformSettings)
  const [chatIndex, setChatIndex] = useState<GenerateChatIndex>(() => readStoredChatIndex())
  const [activeChatId, setActiveChatId] = useState<string | null>(null)
  const [activeImages, setActiveImages] = useState<ImageGeneration[]>([])
  const [history, setHistory] = useState<ImageGeneration[]>([])
  const [historyLoading, setHistoryLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [referenceImages, setReferenceImages] = useState<ReferencePreview[]>([])
  const [previewReference, setPreviewReference] = useState<ReferencePreview | null>(null)
  const [viewer, setViewer] = useState<ImageGeneration | null>(null)
  const [fileInputKey, setFileInputKey] = useState(0)

  const scrollMessagesToBottom = useCallback((behavior: ScrollBehavior = "smooth") => {
    window.requestAnimationFrame(() => {
      const node = messageScrollRef.current
      if (!node) return
      node.scrollTo({ top: node.scrollHeight, behavior })
    })
  }, [])

  useEffect(() => {
    const urls = referenceUrlRef.current
    return () => {
      urls.forEach((url) => URL.revokeObjectURL(url))
      urls.clear()
    }
  }, [])

  useEffect(() => {
    activeImagesRef.current = activeImages
  }, [activeImages])

  const loadHistory = useCallback(async () => {
    setHistoryLoading(true)
    try {
      const { data } = await api.get<{ data: ImageGeneration[] }>("/api/images?limit=30")
      setHistory(data.data ?? [])
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setHistoryLoading(false)
    }
  }, [])

  useEffect(() => {
    const handlePlatformSettingsUpdated = (event: Event) => {
      setPlatform(mergePlatformSettings((event as CustomEvent<PlatformSettings>).detail))
    }

    window.addEventListener(platformSettingsUpdatedEvent, handlePlatformSettingsUpdated)
    Promise.all([
      api.get<{ data: PricingRule[] }>("/api/pricing"),
      api.get<PlatformSettings>("/api/settings/platform"),
    ])
      .then(([pricingResponse, platformResponse]) => {
        setPricing(pricingResponse.data.data ?? [])
        setPlatform(mergePlatformSettings(platformResponse.data))
      })
      .catch((error) => toast.error(errorMessage(error)))
    const timer = window.setTimeout(() => {
      void loadHistory()
    }, 0)
    return () => {
      window.clearTimeout(timer)
      window.removeEventListener(platformSettingsUpdatedEvent, handlePlatformSettingsUpdated)
    }
  }, [loadHistory])

  const cost = useMemo(() => {
    const bucket = resolutionBucket(width, height)
    return pricing.find((rule) => rule.model === "gpt-image-2" && rule.size === bucket && rule.quality === quality)?.credits
  }, [height, pricing, quality, width])

  const sizeError = useMemo(() => validateImageSize(width, height, platform.maxResolutionBucket), [height, platform.maxResolutionBucket, width])
  const bucket = resolutionBucket(width, height)
  const maxEdge = platform.allow4k ? 3840 : 2048
  const maxHeight = maxEdge
  const sizeDraftChanged = widthInput !== String(width) || heightInput !== String(height)
  const current = activeImages.length > 0 ? activeImages[activeImages.length - 1] : null
  const currentId = current?.id
  const currentStatus = current?.status
  const busy = activeImages.some((image) => generationBusy(image))
  const historyThreads = useMemo<HistoryThread[]>(() => {
    const groups = new Map<string, ImageGeneration[]>()
    history.forEach((image) => {
      const chatId = chatIndex.imageToChat[image.id] ?? image.id
      groups.set(chatId, [...(groups.get(chatId) ?? []), image])
    })

    return Array.from(groups.entries())
      .map(([id, images]) => {
        const ordered = orderImagesForChat(id, images, chatIndex)
        const latest = ordered.reduce((next, image) => (createdAtTime(image) > createdAtTime(next) ? image : next), ordered[0])
        return { id, images: ordered, latest }
      })
      .filter((thread): thread is HistoryThread => Boolean(thread.latest))
      .sort((a, b) => createdAtTime(b.latest) - createdAtTime(a.latest))
  }, [chatIndex, history])

  function confirmDimensions(showToast = false) {
    const nextWidth = parseDimensionInput(widthInput)
    const nextHeight = parseDimensionInput(heightInput)
    if (nextWidth == null || nextHeight == null) {
      const message = "请输入有效宽高"
      setSizeDraftError(message)
      if (showToast) toast.error(message)
      return null
    }
    const error = validateImageSize(nextWidth, nextHeight, platform.maxResolutionBucket)
    if (error) {
      setSizeDraftError(error)
      if (showToast) toast.error(error)
      return null
    }
    setSizeDraftError(null)
    setWidth(nextWidth)
    setHeight(nextHeight)
    setWidthInput(String(nextWidth))
    setHeightInput(String(nextHeight))
    return { width: nextWidth, height: nextHeight }
  }

  function rememberChatImages(chatId: string, images: ImageGeneration[]) {
    setChatIndex((state) => {
      const next = upsertChatImages(state, chatId, images.map((image) => image.id))
      writeStoredChatIndex(next)
      return next
    })
    setHistory((state) => {
      const byId = new Map(state.map((item) => [item.id, item]))
      images.forEach((image) => byId.set(image.id, image))
      return Array.from(byId.values()).sort((a, b) => createdAtTime(b) - createdAtTime(a))
    })
  }

  useEffect(() => {
    if (!currentId || currentStatus === "COMPLETED" || currentStatus === "FAILED" || currentStatus === "EXPIRED" || !activeChatId) return
    const timer = window.setInterval(async () => {
      try {
        const { data } = await api.get<{ image: ImageGeneration }>(`/api/images/${currentId}/status`)
        const nextImages = activeImagesRef.current.map((image) => (image.id === data.image.id ? data.image : image))
        setActiveImages(nextImages)
        rememberChatImages(activeChatId, nextImages)
        if (data.image.status === "COMPLETED") {
          toast.success("图片已生成")
          void refreshMe()
          void loadHistory()
          window.clearInterval(timer)
        }
        if (data.image.status === "FAILED") {
          toast.error(data.image.errorMessage ?? "生成失败")
          void refreshMe()
          void loadHistory()
          window.clearInterval(timer)
        }
      } catch (error) {
        toast.error(errorMessage(error))
      }
    }, 2500)
    return () => window.clearInterval(timer)
  }, [activeChatId, currentId, currentStatus, loadHistory, refreshMe])

  useEffect(() => {
    if (!currentId) return
    scrollMessagesToBottom()
  }, [currentId, currentStatus, scrollMessagesToBottom])

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const dimensions = confirmDimensions(true)
    if (!dimensions) {
      return
    }
    const nextBucket = resolutionBucket(dimensions.width, dimensions.height)
    const nextCost = pricing.find((rule) => rule.model === "gpt-image-2" && rule.size === nextBucket && rule.quality === quality)?.credits
    if (nextCost == null) {
      toast.error("当前尺寸和质量没有配置定价")
      return
    }
    setSubmitting(true)
    try {
      const form = new FormData()
      form.append("model", "gpt-image-2")
      form.append("prompt", prompt)
      form.append("size", sizeString(dimensions.width, dimensions.height))
      form.append("quality", quality)
      referenceImages.forEach((image) => form.append("referenceImages", image.file))
      const { data } = await api.post<{ image: ImageGeneration }>("/api/images/generate", form)
      const chatId = activeChatId ?? data.image.id
      const nextImages = [...activeImages, data.image]
      setActiveChatId(chatId)
      setActiveImages(nextImages)
      rememberChatImages(chatId, nextImages)
      setPrompt("")
      clearReferenceImages()
      setFileInputKey((value) => value + 1)
      toast.success("任务已提交")
      void refreshMe()
      void loadHistory()
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
    if (accepted.length === 0) return
    const slots = Math.max(0, 4 - referenceImages.length)
    if (slots === 0) {
      toast.error("最多提交 4 张参考图")
      return
    }
    const selected = accepted.slice(0, slots)
    if (accepted.length > slots) {
      toast.error("最多提交 4 张参考图")
    }
    const previews = selected.map((file) => {
      const url = URL.createObjectURL(file)
      referenceUrlRef.current.add(url)
      return { id: createReferenceId(file), file, url }
    })
    setReferenceImages((state) => [...state, ...previews])
  }

  function removeReferenceImage(id: string) {
    const target = referenceImages.find((image) => image.id === id)
    if (!target) return
    URL.revokeObjectURL(target.url)
    referenceUrlRef.current.delete(target.url)
    setReferenceImages((state) => state.filter((image) => image.id !== id))
    if (previewReference?.id === id) setPreviewReference(null)
  }

  function clearReferenceImages() {
    referenceUrlRef.current.forEach((url) => URL.revokeObjectURL(url))
    referenceUrlRef.current.clear()
    setReferenceImages([])
    setPreviewReference(null)
  }

  function startNewChat() {
    setActiveChatId(null)
    setActiveImages([])
    setPrompt("")
    clearReferenceImages()
    setFileInputKey((value) => value + 1)
  }

  function openHistoryThread(thread: HistoryThread) {
    const images = orderImagesForChat(thread.id, thread.images, chatIndex)
    setActiveChatId(thread.id)
    setActiveImages(images)
    rememberChatImages(thread.id, images)
    setPrompt("")
    clearReferenceImages()
    const latestSize = parseImageSize(thread.latest.size)
    if (latestSize) {
      setWidth(latestSize.width)
      setHeight(latestSize.height)
      setWidthInput(String(latestSize.width))
      setHeightInput(String(latestSize.height))
      setSizeDraftError(null)
    }
    setQuality(thread.latest.quality)
    window.requestAnimationFrame(() => scrollMessagesToBottom("auto"))
  }

  return (
    <>
      <PageHeader
        title="生成工作台"
        action={
          <Badge variant="secondary" className="h-7 px-3 text-sm tabular-nums">
            余额 {user?.balance ?? 0} credits
          </Badge>
        }
      />
      <div className="grid h-[calc(100svh-170px)] min-h-[560px] gap-4 overflow-hidden lg:grid-cols-[220px_minmax(0,1fr)]">
        <aside className="hidden min-h-0 overflow-hidden rounded-lg border bg-background lg:flex lg:flex-col">
          <div className="flex items-center justify-between gap-2 border-b p-3">
            <div className="flex items-center gap-2 text-sm font-medium">
              <History />
              历史
            </div>
            <Button type="button" variant="outline" size="icon-sm" aria-label="新建对话" onClick={startNewChat}>
              <Plus />
            </Button>
          </div>
          <div className="min-h-0 flex-1 overflow-auto p-2">
            {historyLoading ? (
              <div className="px-2 py-3 text-sm text-muted-foreground">加载中</div>
            ) : historyThreads.length === 0 ? (
              <div className="px-2 py-3 text-sm text-muted-foreground">暂无历史</div>
            ) : (
              <div className="flex flex-col gap-1">
                {historyThreads.map((thread) => (
                  <button
                    key={thread.id}
                    type="button"
                    className={cn(
                      "flex min-h-16 flex-col gap-1 rounded-lg px-3 py-2 text-left text-sm transition-colors hover:bg-muted",
                      activeChatId === thread.id && "bg-muted"
                    )}
                    onClick={() => openHistoryThread(thread)}
                  >
                    <span className="line-clamp-2 leading-5">{thread.latest.prompt}</span>
                    <span className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                      <span>{formatDate(thread.latest.createdAt)}</span>
                      <span className="flex items-center gap-1">
                        {thread.images.length > 1 && <Badge variant="outline">{thread.images.length}</Badge>}
                        <Badge variant={thread.latest.status === "COMPLETED" ? "secondary" : "outline"}>{statusLabel(thread.latest.status)}</Badge>
                      </span>
                    </span>
                  </button>
                ))}
              </div>
            )}
          </div>
        </aside>

        <div className="mx-auto flex min-h-0 w-full max-w-5xl flex-col overflow-hidden rounded-lg border bg-background">
          <div className="flex items-center justify-between gap-3 border-b px-4 py-3 lg:hidden">
            <div className="flex items-center gap-2 text-sm font-medium">
              <History />
              最近历史
            </div>
            <Button type="button" variant="outline" size="sm" onClick={startNewChat}>
              <Plus data-icon="inline-start" />
              新建
            </Button>
          </div>

          <div ref={messageScrollRef} className="min-h-0 flex-1 overflow-auto bg-muted/10 p-4 md:p-6">
            {activeImages.length === 0 ? (
              <Empty className="min-h-full">
                <EmptyHeader>
                  <EmptyMedia variant="icon">
                    <ImagePlus />
                  </EmptyMedia>
                  <EmptyTitle>开始一张新图片</EmptyTitle>
                  <EmptyDescription>输入提示词，也可以附上参考图。</EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : (
              <div className="mx-auto flex w-full max-w-3xl flex-col gap-5">
                {activeImages.map((image) => (
                  <div key={image.id} className="flex flex-col gap-5">
                    <div className="flex justify-end">
                      <div className="max-w-[78%] rounded-lg bg-primary px-4 py-3 text-sm text-primary-foreground">
                        <div className="whitespace-pre-wrap leading-6">{image.prompt}</div>
                        <div className="mt-2 flex flex-wrap gap-1">
                          <Badge variant="secondary">{image.size}</Badge>
                          <Badge variant="secondary">{qualityLabel(image.quality)}</Badge>
                          {image.referenceCount > 0 && <Badge variant="secondary">参考图 {image.referenceCount}</Badge>}
                        </div>
                      </div>
                    </div>
                    <div className="flex justify-start">
                      <div className="flex w-full max-w-[82%] flex-col gap-3 rounded-lg border bg-background p-3 shadow-sm">
                        {generationBusy(image) ? (
                          <div className="flex min-h-[220px] flex-col justify-center gap-4 p-3">
                            <div className="flex items-center gap-2 text-sm font-medium">
                              <RefreshCw className="animate-spin" />
                              {image.status === "PROCESSING" ? "正在生成图片" : "正在排队"}
                            </div>
                            <Progress value={image.status === "PROCESSING" ? 66 : 30} />
                            <div className="text-xs text-muted-foreground">任务 ID {image.id}</div>
                          </div>
                        ) : image.status === "COMPLETED" ? (
                          <>
                            <button
                              type="button"
                              className="group flex max-h-[52vh] w-full items-center justify-center overflow-hidden rounded-lg bg-muted/30 outline-none transition-all duration-150 hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 active:scale-[0.99]"
                              aria-label="查看大图"
                              onClick={() => setViewer(image)}
                            >
                              <SecureImage
                                imageId={image.id}
                                alt={image.prompt}
                                maxEdge={1024}
                                className="max-h-[52vh] w-full object-contain transition-transform duration-200 group-hover:scale-[1.01]"
                                onLoad={() => scrollMessagesToBottom()}
                              />
                            </button>
                            <div className="flex flex-wrap items-center justify-between gap-2 text-sm text-muted-foreground">
                              <span>过期倒计时 {expiresIn(image.expiresAt)}</span>
                              <Button variant="outline" size="sm" onClick={() => void downloadImage(image.id)}>
                                <Download data-icon="inline-start" />
                                下载
                              </Button>
                            </div>
                          </>
                        ) : (
                          <div className="rounded-lg border bg-muted/20 p-4 text-sm text-muted-foreground">{image.errorMessage ?? "任务未完成"}</div>
                        )}
                      </div>
                    </div>
                  </div>
                ))}
                {busy && (
                  <div className="flex justify-start">
                    <div className="rounded-lg border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">当前会话有任务正在生成，完成后可以继续发送。</div>
                  </div>
                )}
              </div>
            )}
          </div>

          <form className="flex shrink-0 flex-col gap-3 border-t bg-background p-3" onSubmit={submit}>
            {referenceImages.length > 0 && (
              <div className="mx-auto flex w-full max-w-3xl flex-wrap gap-2">
                {referenceImages.map((image) => (
                  <div key={image.id} className="group relative size-14 overflow-hidden rounded-lg border bg-muted">
                    <button type="button" className="h-full w-full" aria-label="查看参考图" onClick={() => setPreviewReference(image)}>
                      <img src={image.url} alt={image.file.name} className="h-full w-full object-cover" />
                    </button>
                    <Button
                      type="button"
                      variant="secondary"
                      size="icon-xs"
                      className="absolute top-1 right-1 bg-background/90 opacity-0 shadow-sm transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
                      aria-label="移除参考图"
                      onClick={() => removeReferenceImage(image.id)}
                    >
                      <X />
                    </Button>
                  </div>
                ))}
              </div>
            )}

            <div className="mx-auto flex w-full max-w-3xl flex-col gap-2 rounded-lg border bg-background p-2.5">
              <FieldGroup className="gap-2">
                <Field>
                  <FieldLabel htmlFor="prompt" className="sr-only">
                    提示词
                  </FieldLabel>
                  <Textarea
                    id="prompt"
                    value={prompt}
                    onChange={(event) => setPrompt(event.target.value)}
                    rows={2}
                    className="resize-none border-0 bg-transparent p-0 shadow-none focus-visible:ring-0"
                    placeholder="描述你想生成的图片"
                    required
                  />
                </Field>
              </FieldGroup>

              <div className="border-t pt-2">
                <FieldGroup className="flex-row flex-wrap items-center gap-2">
                  <Field orientation="horizontal" className="w-auto shrink-0 items-center gap-2">
                    <FieldLabel className="!flex-none text-xs text-muted-foreground">尺寸</FieldLabel>
                    <div className="flex items-center gap-1.5">
                      <Input
                        aria-label="宽度"
                        type="number"
                        min={16}
                        max={maxEdge}
                        step={16}
                        value={widthInput}
                        className="h-7 w-20 px-2 text-sm"
                        onBlur={() => confirmDimensions()}
                        onChange={(event) => {
                          setSizeDraftError(null)
                          setWidthInput(event.target.value)
                        }}
                        onKeyDown={(event) => {
                          if (event.key !== "Enter") return
                          event.preventDefault()
                          confirmDimensions()
                          event.currentTarget.blur()
                        }}
                      />
                      <span className="text-xs text-muted-foreground">x</span>
                      <Input
                        aria-label="高度"
                        type="number"
                        min={16}
                        max={maxHeight}
                        step={16}
                        value={heightInput}
                        className="h-7 w-20 px-2 text-sm"
                        onBlur={() => confirmDimensions()}
                        onChange={(event) => {
                          setSizeDraftError(null)
                          setHeightInput(event.target.value)
                        }}
                        onKeyDown={(event) => {
                          if (event.key !== "Enter") return
                          event.preventDefault()
                          confirmDimensions()
                          event.currentTarget.blur()
                        }}
                      />
                    </div>
                    {(sizeDraftError || !sizeDraftChanged) && (
                      <FieldDescription className={cn("whitespace-nowrap text-xs", sizeDraftError && "text-destructive")}>
                        {sizeDraftError ?? sizeError ?? `${bucket.toUpperCase()} 档`}
                      </FieldDescription>
                    )}
                  </Field>
                  <Field orientation="horizontal" className="w-auto shrink-0 items-center gap-2">
                    <FieldLabel className="!flex-none text-xs text-muted-foreground">质量</FieldLabel>
                    <Select value={quality} onValueChange={(value) => setQuality(value ?? quality)}>
                      <SelectTrigger className="h-7 w-24">
                        <SelectValue>{qualityLabel(quality)}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {qualityOptions.map((item) => (
                            <SelectItem key={item.value} value={item.value}>
                              {item.label}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field orientation="horizontal" className="w-auto shrink-0 items-center gap-2">
                    <FieldLabel htmlFor="reference-images" className="!flex-none text-xs text-muted-foreground">
                      参考图
                    </FieldLabel>
                    <Button
                      type="button"
                      variant="outline"
                      size="icon-sm"
                      className="rounded-full"
                      aria-label="上传参考图"
                      disabled={referenceImages.length >= 4}
                      onClick={() => fileInputRef.current?.click()}
                    >
                      <Upload />
                    </Button>
                    <Input
                      ref={fileInputRef}
                      key={fileInputKey}
                      id="reference-images"
                      type="file"
                      accept="image/png,image/jpeg,image/webp"
                      multiple
                      className="hidden"
                      onChange={(event) => {
                        addReferenceFiles(event.target.files)
                        event.currentTarget.value = ""
                      }}
                    />
                  </Field>
                  <div className="ml-auto flex shrink-0 items-center gap-2">
                    <Badge variant="secondary">{sizeDraftChanged ? "-" : (cost ?? "-")} credits</Badge>
                    <Button
                      size="sm"
                      disabled={submitting || busy || !prompt.trim() || !!sizeDraftError || (!sizeDraftChanged && (!!sizeError || cost == null))}
                      type="submit"
                    >
                      {submitting || busy ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <SendHorizontal data-icon="inline-start" />}
                      {submitting ? "提交中" : busy ? "生成中" : "发送"}
                    </Button>
                  </div>
                </FieldGroup>
              </div>
            </div>
          </form>
        </div>
      </div>
      <Dialog open={!!previewReference} onOpenChange={(open) => !open && setPreviewReference(null)}>
        <DialogContent className="max-h-[96svh] max-w-[96vw] overflow-hidden sm:max-w-4xl">
          <DialogHeader>
            <DialogTitle>查看参考图</DialogTitle>
          </DialogHeader>
          {previewReference && (
            <div className="flex max-h-[82vh] items-center justify-center overflow-hidden rounded-lg border bg-muted/30 p-3">
              <img src={previewReference.url} alt={previewReference.file.name} className="max-h-[78vh] w-full rounded-md object-contain" />
            </div>
          )}
        </DialogContent>
      </Dialog>
      <ImageViewerDialog image={viewer} open={!!viewer} onOpenChange={(open) => !open && setViewer(null)} />
    </>
  )
}
