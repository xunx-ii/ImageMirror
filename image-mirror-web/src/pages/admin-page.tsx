import { useCallback, useEffect, useMemo, useState } from "react"
import type { ChangeEvent, FormEvent } from "react"
import {
  BadgeCent,
  Ban,
  Bell,
  Copy,
  CreditCard,
  Eye,
  Gift,
  ImagePlus,
  Pencil,
  Plus,
  RefreshCw,
  Save,
  Search,
  Shield,
  ToggleLeft,
  ToggleRight,
  Trash2,
  Users,
  type LucideIcon,
} from "lucide-react"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { emitSiteContentUpdated } from "@/lib/content"
import { formatDate } from "@/lib/format"
import { resolutionBuckets } from "@/lib/image-size"
import { renderMarkdown } from "@/lib/markdown"
import { defaultPlatformSettings, emitPlatformSettingsUpdated, mergePlatformSettings } from "@/lib/platform"
import { useAuthStore } from "@/stores/auth"
import type { AdminOverview, ContentAsset, EPaySettings, OpenAIEndpoint, OpenAISettings, PlatformSettings, PricingRule, RedemptionCode, SiteContent, UsageLog, UsageLogList, UsageRetention, User } from "@/types"

const qualities = ["low", "medium", "high", "auto"]
type UsageDetailKind = "prompt" | "result"

function qualityLabel(value: string) {
  switch (value) {
    case "low":
      return "低"
    case "medium":
      return "中"
    case "high":
      return "高"
    case "auto":
      return "自动"
    default:
      return value
  }
}

export function AdminPage() {
  const currentUser = useAuthStore((state) => state.user)
  const [overview, setOverview] = useState<AdminOverview | null>(null)
  const [users, setUsers] = useState<User[]>([])
  const [pricing, setPricing] = useState<PricingRule[]>([])
  const [codes, setCodes] = useState<RedemptionCode[]>([])
  const [selectedCodes, setSelectedCodes] = useState<string[]>([])
  const [openAI, setOpenAI] = useState<OpenAISettings | null>(null)
  const [epay, setEPay] = useState<EPaySettings | null>(null)
  const [platform, setPlatform] = useState<PlatformSettings>(defaultPlatformSettings)
  const [docs, setDocs] = useState<SiteContent | null>(null)
  const [announcement, setAnnouncement] = useState<SiteContent | null>(null)

  const [model, setModel] = useState("gpt-image-2")
  const [size, setSize] = useState("1k")
  const [quality, setQuality] = useState("medium")
  const [credits, setCredits] = useState(8)
  const [editingPricingId, setEditingPricingId] = useState<string | null>(null)
  const [pricingDialogOpen, setPricingDialogOpen] = useState(false)

  const [codeCredits, setCodeCredits] = useState(100)
  const [codeCount, setCodeCount] = useState(1)
  const [codeExpiresAt, setCodeExpiresAt] = useState("")
  const [codeDialogOpen, setCodeDialogOpen] = useState(false)

  const [docsTitle, setDocsTitle] = useState("文档")
  const [docsBody, setDocsBody] = useState("")
  const [docsActive, setDocsActive] = useState(true)
  const [announcementTitle, setAnnouncementTitle] = useState("公告")
  const [announcementBody, setAnnouncementBody] = useState("")
  const [announcementActive, setAnnouncementActive] = useState(false)
  const [siteTitle, setSiteTitle] = useState(defaultPlatformSettings.siteTitle)
  const [siteSubtitle, setSiteSubtitle] = useState(defaultPlatformSettings.siteSubtitle)

  const [openAIEndpointName, setOpenAIEndpointName] = useState("默认节点")
  const [openAIBaseUrl, setOpenAIBaseUrl] = useState("https://api.openai.com")
  const [openAIKey, setOpenAIKey] = useState("")
  const [openAIEnabled, setOpenAIEnabled] = useState(true)
  const [openAISchedulable, setOpenAISchedulable] = useState(true)
  const [openAIPriority, setOpenAIPriority] = useState(100)
  const [editingOpenAIEndpointId, setEditingOpenAIEndpointId] = useState<string | null>(null)
  const [openAIEndpointDialogOpen, setOpenAIEndpointDialogOpen] = useState(false)
  const [epayGateway, setEPayGateway] = useState("https://pay.example.com")
  const [epayPID, setEPayPID] = useState("")
  const [epayKey, setEPayKey] = useState("")
  const [epayName, setEPayName] = useState("ImageMirror credits")
  const [epayCreditsPerYuan, setEPayCreditsPerYuan] = useState(100)
  const [epayEnabled, setEPayEnabled] = useState(false)
  const [adjustments, setAdjustments] = useState<Record<string, number>>({})
  const [activeTab, setActiveTab] = useState("overview")
  const [usageLogs, setUsageLogs] = useState<UsageLog[]>([])
  const [usageTotal, setUsageTotal] = useState(0)
  const [usageLimit] = useState(20)
  const [usageOffset, setUsageOffset] = useState(0)
  const [usageUserQuery, setUsageUserQuery] = useState("")
  const [usagePromptQuery, setUsagePromptQuery] = useState("")
  const [usageSource, setUsageSource] = useState("all")
  const [usageSuccess, setUsageSuccess] = useState("all")
  const [usageAfter, setUsageAfter] = useState("")
  const [usageBefore, setUsageBefore] = useState("")
  const [usageFilters, setUsageFilters] = useState({
    user: "",
    prompt: "",
    source: "all",
    success: "all",
    after: "",
    before: "",
  })
  const [usageRetentionDays, setUsageRetentionDays] = useState(90)
  const [usageDeleteDays, setUsageDeleteDays] = useState(90)
  const [usageDeleteBefore, setUsageDeleteBefore] = useState("")
  const [usageRetentionDialogOpen, setUsageRetentionDialogOpen] = useState(false)
  const [usageDeleteDialogOpen, setUsageDeleteDialogOpen] = useState(false)
  const [usageDetailLog, setUsageDetailLog] = useState<UsageLog | null>(null)
  const [usageDetailKind, setUsageDetailKind] = useState<UsageDetailKind>("prompt")
  const [usageLoading, setUsageLoading] = useState(false)

  const selectedCodeSet = useMemo(() => new Set(selectedCodes), [selectedCodes])
  const allCodesSelected = codes.length > 0 && selectedCodes.length === codes.length
  const enabledResolutionBuckets = useMemo(() => resolutionBuckets.filter((item) => platform.allow4k || item !== "4k"), [platform.allow4k])
  const visiblePricing = useMemo(() => pricing.filter((rule) => platform.allow4k || rule.size !== "4k"), [platform.allow4k, pricing])
  const usagePage = Math.floor(usageOffset / usageLimit) + 1
  const usageTotalPages = Math.max(1, Math.ceil(usageTotal / usageLimit))
  const usageCanPrev = usageOffset > 0
  const usageCanNext = usageOffset + usageLimit < usageTotal

  const load = useCallback(async () => {
    try {
      const [statsResponse, usersResponse, pricingResponse, codesResponse, openAIResponse, epayResponse, platformResponse, docsResponse, announcementResponse] = await Promise.all([
        api.get<AdminOverview>("/api/admin/stats/overview"),
        api.get<{ data: User[] }>("/api/admin/users?limit=100"),
        api.get<{ data: PricingRule[] }>("/api/admin/pricing"),
        api.get<{ data: RedemptionCode[] }>("/api/admin/redemption-codes?limit=100"),
        api.get<OpenAISettings>("/api/admin/config/openai"),
        api.get<EPaySettings>("/api/admin/config/epay"),
        api.get<PlatformSettings>("/api/admin/config/platform"),
        api.get<SiteContent>("/api/admin/content/docs"),
        api.get<SiteContent>("/api/admin/content/announcement"),
      ])
      setOverview(statsResponse.data)
      setUsers(usersResponse.data.data ?? [])
      setPricing(pricingResponse.data.data ?? [])
      setCodes(codesResponse.data.data ?? [])
      setSelectedCodes((state) => state.filter((id) => (codesResponse.data.data ?? []).some((code) => code.id === id)))

      setOpenAI(openAIResponse.data)
      if (!editingOpenAIEndpointId && (openAIResponse.data.endpoints?.length ?? 0) === 0) {
        setOpenAIBaseUrl(openAIResponse.data.openaiBaseUrl || "https://api.openai.com")
      }

      setEPay(epayResponse.data)
      setEPayGateway(epayResponse.data.gateway || "https://pay.example.com")
      setEPayPID(epayResponse.data.pid ?? "")
      setEPayName(epayResponse.data.name || "ImageMirror credits")
      setEPayCreditsPerYuan(epayResponse.data.creditsPerYuan || 100)
      setEPayEnabled(epayResponse.data.enabled)
      const nextPlatform = mergePlatformSettings(platformResponse.data)
      setPlatform(nextPlatform)
      setSiteTitle(nextPlatform.siteTitle)
      setSiteSubtitle(nextPlatform.siteSubtitle)
      setSize((value) => (!nextPlatform.allow4k && value === "4k" ? "2k" : value))

      setDocs(docsResponse.data)
      setDocsTitle(docsResponse.data.title || "文档")
      setDocsBody(docsResponse.data.body || "")
      setDocsActive(docsResponse.data.isActive)

      setAnnouncement(announcementResponse.data)
      setAnnouncementTitle(announcementResponse.data.title || "公告")
      setAnnouncementBody(announcementResponse.data.body || "")
      setAnnouncementActive(announcementResponse.data.isActive)
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }, [editingOpenAIEndpointId])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void load()
    }, 0)
    return () => window.clearTimeout(timer)
  }, [load])

  const loadUsageLogs = useCallback(async () => {
    setUsageLoading(true)
    try {
      const params = new URLSearchParams({
        limit: String(usageLimit),
        offset: String(usageOffset),
      })
      if (usageFilters.user.trim()) params.set("user", usageFilters.user.trim())
      if (usageFilters.prompt.trim()) params.set("prompt", usageFilters.prompt.trim())
      if (usageFilters.source !== "all") params.set("source", usageFilters.source)
      if (usageFilters.success !== "all") params.set("success", usageFilters.success)
      if (usageFilters.after) params.set("after", new Date(usageFilters.after).toISOString())
      if (usageFilters.before) params.set("before", new Date(usageFilters.before).toISOString())
      const [logsResponse, retentionResponse] = await Promise.all([
        api.get<UsageLogList>(`/api/admin/usage-logs?${params.toString()}`),
        api.get<UsageRetention>("/api/admin/usage-logs/retention"),
      ])
      setUsageLogs(logsResponse.data.data ?? [])
      setUsageTotal(logsResponse.data.total ?? 0)
      setUsageRetentionDays(retentionResponse.data.days)
      setUsageDeleteDays((value) => (value > 0 ? value : retentionResponse.data.days))
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setUsageLoading(false)
    }
  }, [usageFilters, usageLimit, usageOffset])

  useEffect(() => {
    if (activeTab !== "usage") return
    const timer = window.setTimeout(() => {
      void loadUsageLogs()
    }, 0)
    return () => window.clearTimeout(timer)
  }, [activeTab, loadUsageLogs])

  async function savePricing(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    try {
      const payload = {
        model,
        size,
        quality,
        credits,
        isActive: true,
      }
      if (editingPricingId) {
        await api.put(`/api/admin/pricing/${editingPricingId}`, payload)
      } else {
        await api.post("/api/admin/pricing", payload)
      }
      toast.success(editingPricingId ? "定价已更新" : "定价已新增")
      resetPricingForm()
      setPricingDialogOpen(false)
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function deletePricing(rule: PricingRule) {
    try {
      await api.delete(`/api/admin/pricing/${rule.id}`)
      toast.success("定价已删除")
      if (editingPricingId === rule.id) resetPricingForm()
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function toggle4K() {
    const allow4k = !platform.allow4k
    try {
      const { data } = await api.put<PlatformSettings>("/api/admin/config/platform", {
        maxResolutionBucket: allow4k ? "4k" : "2k",
      })
      const nextPlatform = mergePlatformSettings(data)
      setPlatform(nextPlatform)
      setSiteTitle(nextPlatform.siteTitle)
      setSiteSubtitle(nextPlatform.siteSubtitle)
      if (!nextPlatform.allow4k && size === "4k") {
        setSize("2k")
        setEditingPricingId(null)
      }
      emitPlatformSettingsUpdated(nextPlatform)
      toast.success(nextPlatform.allow4k ? "已启用 4K 图片" : "已禁用 4K，最大支持 2K")
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function generateCodes(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    try {
      const payload: { credits: number; count: number; expiresAt?: string } = {
        credits: codeCredits,
        count: codeCount,
      }
      if (codeExpiresAt) {
        payload.expiresAt = new Date(codeExpiresAt).toISOString()
      }
      const { data } = await api.post<{ data: RedemptionCode[] }>("/api/admin/redemption-codes", payload)
      toast.success(`已生成 ${data.data.length} 个兑换码`)
      setSelectedCodes([])
      setCodeExpiresAt("")
      setCodeDialogOpen(false)
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function bulkCodes(action: "disable" | "delete") {
    if (selectedCodes.length === 0) return
    try {
      const { data } = await api.post<{ count: number }>("/api/admin/redemption-codes/bulk", {
        ids: selectedCodes,
        action,
      })
      toast.success(action === "disable" ? `已停用 ${data.count} 个兑换码` : `已删除 ${data.count} 个兑换码`)
      setSelectedCodes([])
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function saveContent(key: "docs" | "announcement", event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const isDocs = key === "docs"
    try {
      const payload = {
        title: isDocs ? docsTitle : announcementTitle,
        body: isDocs ? docsBody : announcementBody,
        isActive: isDocs ? docsActive : announcementActive,
      }
      const { data } = await api.put<SiteContent>(`/api/admin/content/${key}`, payload)
      if (isDocs) {
        setDocs(data)
        setDocsTitle(data.title)
        setDocsBody(data.body)
        setDocsActive(data.isActive)
      } else {
        setAnnouncement(data)
        setAnnouncementTitle(data.title)
        setAnnouncementBody(data.body)
        setAnnouncementActive(data.isActive)
      }
      emitSiteContentUpdated(data)
      toast.success(isDocs ? "文档已保存" : "公告已保存")
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function uploadDocsImage(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ""
    if (!file) return
    try {
      const form = new FormData()
      form.append("file", file)
      const { data } = await api.post<{ asset: ContentAsset; markdown: string }>("/api/admin/content/docs/assets", form)
      setDocsBody((value) => (value.trim() ? `${value}\n\n${data.markdown}` : data.markdown))
      toast.success("图片已上传")
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function refreshOpenAISettings() {
    const { data } = await api.get<OpenAISettings>("/api/admin/config/openai")
    setOpenAI(data)
    return data
  }

  async function savePlatform(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    try {
      const { data } = await api.put<PlatformSettings>("/api/admin/config/platform", {
        maxResolutionBucket: platform.maxResolutionBucket,
        siteTitle,
        siteSubtitle,
      })
      const nextPlatform = mergePlatformSettings(data)
      setPlatform(nextPlatform)
      setSiteTitle(nextPlatform.siteTitle)
      setSiteSubtitle(nextPlatform.siteSubtitle)
      emitPlatformSettingsUpdated(nextPlatform)
      toast.success("平台信息已保存")
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function saveOpenAIEndpoint(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    try {
      const payload: {
        name: string
        baseUrl: string
        apiKey?: string
        enabled: boolean
        schedulable: boolean
        priority: number
      } = {
        name: openAIEndpointName.trim(),
        baseUrl: openAIBaseUrl.trim(),
        enabled: openAIEnabled,
        schedulable: openAISchedulable,
        priority: openAIPriority,
      }
      if (openAIKey.trim()) {
        payload.apiKey = openAIKey.trim()
      }
      if (editingOpenAIEndpointId) {
        await api.put(`/api/admin/config/openai/endpoints/${editingOpenAIEndpointId}`, payload)
      } else {
        await api.post("/api/admin/config/openai/endpoints", payload)
      }
      await refreshOpenAISettings()
      resetOpenAIForm()
      setOpenAIEndpointDialogOpen(false)
      toast.success(editingOpenAIEndpointId ? "API 节点已更新" : "API 节点已新增")
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  function editOpenAIEndpoint(endpoint: OpenAIEndpoint) {
    setEditingOpenAIEndpointId(endpoint.id)
    setOpenAIEndpointName(endpoint.name)
    setOpenAIBaseUrl(endpoint.baseUrl)
    setOpenAIKey("")
    setOpenAIEnabled(endpoint.enabled)
    setOpenAISchedulable(endpoint.schedulable)
    setOpenAIPriority(endpoint.priority)
    setOpenAIEndpointDialogOpen(true)
  }

  function resetOpenAIForm() {
    setEditingOpenAIEndpointId(null)
    setOpenAIEndpointName("默认节点")
    setOpenAIBaseUrl("https://api.openai.com")
    setOpenAIKey("")
    setOpenAIEnabled(true)
    setOpenAISchedulable(true)
    setOpenAIPriority(100)
  }

  function openNewOpenAIEndpointDialog() {
    resetOpenAIForm()
    setOpenAIEndpointDialogOpen(true)
  }

  function closeOpenAIEndpointDialog() {
    setOpenAIEndpointDialogOpen(false)
    resetOpenAIForm()
  }

  async function deleteOpenAIEndpoint(endpoint: OpenAIEndpoint) {
    if (!window.confirm(`确认删除 API 节点 ${endpoint.name}？`)) return
    try {
      await api.delete(`/api/admin/config/openai/endpoints/${endpoint.id}`)
      if (editingOpenAIEndpointId === endpoint.id) resetOpenAIForm()
      await refreshOpenAISettings()
      toast.success("API 节点已删除")
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function resetOpenAIEndpoint(endpoint: OpenAIEndpoint) {
    try {
      await api.post(`/api/admin/config/openai/endpoints/${endpoint.id}/reset`)
      await refreshOpenAISettings()
      toast.success("熔断状态已恢复")
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function saveEPay(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    try {
      const payload: {
        gateway: string
        pid: string
        key?: string
        name: string
        creditsPerYuan: number
        enabled: boolean
      } = {
        gateway: epayGateway.trim(),
        pid: epayPID.trim(),
        name: epayName.trim(),
        creditsPerYuan: epayCreditsPerYuan,
        enabled: epayEnabled,
      }
      if (epayKey.trim()) {
        payload.key = epayKey.trim()
      }
      const { data } = await api.put<EPaySettings>("/api/admin/config/epay", payload)
      setEPay(data)
      setEPayGateway(data.gateway)
      setEPayPID(data.pid)
      setEPayName(data.name)
      setEPayCreditsPerYuan(data.creditsPerYuan)
      setEPayEnabled(data.enabled)
      setEPayKey("")
      toast.success("易支付配置已保存")
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function adjustBalance(userId: string) {
    const amount = adjustments[userId] ?? 0
    if (!amount) return
    try {
      await api.post(`/api/admin/users/${userId}/adjust-balance`, {
        amount,
        description: "admin adjustment",
      })
      toast.success("余额已调整")
      setAdjustments((state) => ({ ...state, [userId]: 0 }))
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function updateUserStatus(user: User, status: "ACTIVE" | "SUSPENDED") {
    try {
      const { data } = await api.put<{ user: User }>(`/api/admin/users/${user.id}/status`, { status })
      setUsers((state) => state.map((item) => (item.id === user.id ? data.user : item)))
      toast.success(status === "ACTIVE" ? "用户已启用" : "用户已停用")
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function deleteUser(user: User) {
    if (!window.confirm(`确认删除用户 ${user.email}？`)) return
    try {
      await api.delete(`/api/admin/users/${user.id}`)
      toast.success("用户已删除")
      setUsers((state) => state.filter((item) => item.id !== user.id))
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  function searchUsageLogs(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setUsageOffset(0)
    setUsageFilters({
      user: usageUserQuery,
      prompt: usagePromptQuery,
      source: usageSource,
      success: usageSuccess,
      after: usageAfter,
      before: usageBefore,
    })
  }

  function openUsageDetail(log: UsageLog, kind: UsageDetailKind) {
    setUsageDetailLog(log)
    setUsageDetailKind(kind)
  }

  function closeUsageDetail() {
    setUsageDetailLog(null)
  }

  async function saveUsageRetention(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    try {
      const { data } = await api.put<UsageRetention>("/api/admin/usage-logs/retention", {
        days: usageRetentionDays,
      })
      setUsageRetentionDays(data.days)
      setUsageRetentionDialogOpen(false)
      toast.success(data.days === 0 ? "已关闭自动清理" : `自动清理已设置为保留 ${data.days} 天`)
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  async function deleteUsageLogs(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const payload: { before?: string; days?: number } = {}
    let label = `${usageDeleteDays} 天前`
    if (usageDeleteBefore) {
      const before = new Date(usageDeleteBefore)
      if (Number.isNaN(before.getTime())) {
        toast.error("删除截止时间无效")
        return
      }
      payload.before = before.toISOString()
      label = `${before.toLocaleString("zh-CN")} 之前`
    } else if (usageDeleteDays > 0) {
      payload.days = usageDeleteDays
    } else {
      toast.error("请输入大于 0 的天数")
      return
    }
    if (!window.confirm(`确认删除 ${label} 的用量日志？`)) return
    try {
      const { data } = await api.delete<{ count: number }>("/api/admin/usage-logs", {
        data: payload,
      })
      toast.success(`已删除 ${data.count} 条日志`)
      setUsageDeleteDialogOpen(false)
      await loadUsageLogs()
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  function editPricing(rule: PricingRule) {
    setEditingPricingId(rule.id)
    setModel(rule.model)
    setSize(rule.size)
    setQuality(rule.quality)
    setCredits(rule.credits)
    setPricingDialogOpen(true)
  }

  function resetPricingForm() {
    setEditingPricingId(null)
    setModel("gpt-image-2")
    setSize("1k")
    setQuality("medium")
    setCredits(8)
  }

  function openNewPricingDialog() {
    resetPricingForm()
    setPricingDialogOpen(true)
  }

  function closePricingDialog() {
    setPricingDialogOpen(false)
    resetPricingForm()
  }

  function closeCodeDialog() {
    setCodeDialogOpen(false)
  }

  function toggleCode(id: string) {
    setSelectedCodes((state) => (state.includes(id) ? state.filter((item) => item !== id) : [...state, id]))
  }

  function toggleAllCodes() {
    setSelectedCodes(allCodesSelected ? [] : codes.map((code) => code.id))
  }

  function copySelectedCodes() {
    const text = codes
      .filter((code) => selectedCodeSet.has(code.id))
      .map((code) => code.code)
      .join("\n")
    if (!text) return
    void navigator.clipboard.writeText(text)
    toast.success("已复制兑换码")
  }

  return (
    <>
      <PageHeader
        title="管理"
        action={
          <Button variant="outline" onClick={load}>
            <RefreshCw data-icon="inline-start" />
            刷新
          </Button>
        }
      />

      <Tabs value={activeTab} onValueChange={setActiveTab} className="gap-4">
        <TabsList className="grid h-auto w-full grid-cols-2 md:grid-cols-5 xl:grid-cols-9">
          <TabsTrigger value="overview">概览</TabsTrigger>
          <TabsTrigger value="users">用户</TabsTrigger>
          <TabsTrigger value="usage">用量</TabsTrigger>
          <TabsTrigger value="pricing">定价</TabsTrigger>
          <TabsTrigger value="codes">兑换码</TabsTrigger>
          <TabsTrigger value="docs">文档</TabsTrigger>
          <TabsTrigger value="announcement">公告</TabsTrigger>
          <TabsTrigger value="openai">OpenAI</TabsTrigger>
          <TabsTrigger value="payment">支付</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="flex flex-col gap-4">
          <div className="grid gap-4 md:grid-cols-4">
            <AdminMetric icon={Users} label="用户" value={overview?.users ?? 0} />
            <AdminMetric icon={ImagePlus} label="图片任务" value={overview?.images ?? 0} />
            <AdminMetric icon={Shield} label="已完成" value={overview?.completed ?? 0} />
            <AdminMetric icon={BadgeCent} label="已消耗积分" value={overview?.creditsConsumed ?? 0} />
          </div>
          <Card>
            <CardHeader>
              <CardTitle>平台状态</CardTitle>
              <CardDescription>生成任务、用户和积分消耗的当前汇总。</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-wrap gap-2">
              <Badge variant="secondary">OpenAI {openAI?.hasOpenaiApiKey ? "已配置" : "未配置"}</Badge>
              <Badge variant="secondary">支付 {epay?.enabled ? "已启用" : "未启用"}</Badge>
              <Badge variant="secondary">规则 {pricing.length}</Badge>
              <Badge variant="secondary">最大 {platform.maxResolutionBucket.toUpperCase()}</Badge>
              <Badge variant="secondary">兑换码 {codes.length}</Badge>
              <Badge variant="secondary">用户 {users.length}</Badge>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>平台信息</CardTitle>
            </CardHeader>
            <CardContent>
              <form className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]" onSubmit={savePlatform}>
                <Field>
                  <FieldLabel htmlFor="site-title">标题</FieldLabel>
                  <Input id="site-title" value={siteTitle} onChange={(event) => setSiteTitle(event.target.value)} />
                </Field>
                <Field>
                  <FieldLabel htmlFor="site-subtitle">副标题</FieldLabel>
                  <Input id="site-subtitle" value={siteSubtitle} onChange={(event) => setSiteSubtitle(event.target.value)} />
                </Field>
                <div className="flex items-end">
                  <Button type="submit" className="w-full lg:w-auto">
                    <Save data-icon="inline-start" />
                    保存
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="users">
          <Card>
            <CardHeader>
              <CardTitle>用户</CardTitle>
              <CardDescription>查看用户角色和余额，并进行人工加减款。</CardDescription>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>邮箱</TableHead>
                    <TableHead>角色</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>余额</TableHead>
                    <TableHead>注册时间</TableHead>
                    <TableHead>调整</TableHead>
                    <TableHead>操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {users.map((user) => {
                    const isSelf = currentUser?.id === user.id
                    return (
                      <TableRow key={user.id}>
                        <TableCell>{user.email}</TableCell>
                        <TableCell>
                          <Badge variant={user.role === "ADMIN" ? "secondary" : "outline"}>{user.role}</Badge>
                        </TableCell>
                        <TableCell>
                          <Badge variant={user.status === "ACTIVE" ? "secondary" : "destructive"}>{user.status}</Badge>
                        </TableCell>
                        <TableCell className="tabular-nums">{user.balance}</TableCell>
                        <TableCell>{formatDate(user.createdAt)}</TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <Input
                              className="w-28"
                              type="number"
                              value={adjustments[user.id] ?? 0}
                              onChange={(event) =>
                                setAdjustments((state) => ({
                                  ...state,
                                  [user.id]: Number(event.target.value),
                                }))
                              }
                            />
                            <Button variant="outline" onClick={() => void adjustBalance(user.id)}>
                              保存
                            </Button>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-wrap gap-2">
                            {user.status === "ACTIVE" ? (
                              <Button variant="outline" disabled={isSelf} onClick={() => void updateUserStatus(user, "SUSPENDED")}>
                                <Ban data-icon="inline-start" />
                                停用
                              </Button>
                            ) : (
                              <Button variant="outline" onClick={() => void updateUserStatus(user, "ACTIVE")}>
                                启用
                              </Button>
                            )}
                            <Button variant="outline" disabled={isSelf} onClick={() => void deleteUser(user)}>
                              <Trash2 data-icon="inline-start" />
                              删除
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="usage" className="flex flex-col gap-4">
          <Card>
            <CardHeader>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <CardTitle>用量统计</CardTitle>
                  <CardDescription>查看用户请求、IP、耗时、消耗和生成结果。</CardDescription>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button variant="outline" onClick={() => setUsageRetentionDialogOpen(true)}>
                    <Save data-icon="inline-start" />
                    自动清理
                  </Button>
                  <Button variant="outline" onClick={() => setUsageDeleteDialogOpen(true)}>
                    <Trash2 data-icon="inline-start" />
                    删除日志
                  </Button>
                  <Button variant="outline" onClick={() => void loadUsageLogs()} disabled={usageLoading}>
                    <RefreshCw data-icon="inline-start" />
                    刷新
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <form className="grid gap-3 lg:grid-cols-[minmax(160px,1fr)_minmax(160px,1fr)_130px_130px_170px_170px_auto]" onSubmit={searchUsageLogs}>
                <Field>
                  <FieldLabel htmlFor="usage-user">用户</FieldLabel>
                  <Input id="usage-user" placeholder="邮箱或用户 ID" value={usageUserQuery} onChange={(event) => setUsageUserQuery(event.target.value)} />
                </Field>
                <Field>
                  <FieldLabel htmlFor="usage-prompt">提示词</FieldLabel>
                  <Input id="usage-prompt" placeholder="搜索提示词" value={usagePromptQuery} onChange={(event) => setUsagePromptQuery(event.target.value)} />
                </Field>
                <Field>
                  <FieldLabel htmlFor="usage-source">来源</FieldLabel>
                  <Select value={usageSource} onValueChange={(value) => setUsageSource(value ?? "all")}>
                    <SelectTrigger id="usage-source" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="all">全部</SelectItem>
                        <SelectItem value="WEB">前台</SelectItem>
                        <SelectItem value="API">API</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
                <Field>
                  <FieldLabel htmlFor="usage-success">结果</FieldLabel>
                  <Select value={usageSuccess} onValueChange={(value) => setUsageSuccess(value ?? "all")}>
                    <SelectTrigger id="usage-success" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="all">全部</SelectItem>
                        <SelectItem value="true">成功</SelectItem>
                        <SelectItem value="false">失败</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
                <Field>
                  <FieldLabel htmlFor="usage-after">开始</FieldLabel>
                  <Input id="usage-after" type="datetime-local" value={usageAfter} onChange={(event) => setUsageAfter(event.target.value)} />
                </Field>
                <Field>
                  <FieldLabel htmlFor="usage-before">结束</FieldLabel>
                  <Input id="usage-before" type="datetime-local" value={usageBefore} onChange={(event) => setUsageBefore(event.target.value)} />
                </Field>
                <div className="flex items-end">
                  <Button type="submit" className="w-full lg:w-auto">
                    <Search data-icon="inline-start" />
                    搜索
                  </Button>
                </div>
              </form>

              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>时间</TableHead>
                    <TableHead>用户</TableHead>
                    <TableHead>IP / 来源</TableHead>
                    <TableHead>调用记录</TableHead>
                    <TableHead>耗时</TableHead>
                    <TableHead>消耗</TableHead>
                    <TableHead>提示词</TableHead>
                    <TableHead>结果</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {usageLogs.map((log) => (
                    <TableRow key={log.id}>
                      <TableCell>{formatDate(log.createdAt)}</TableCell>
                      <TableCell>
                        <div className="flex min-w-[180px] flex-col gap-1">
                          <span className="truncate font-medium">{log.userEmail || "-"}</span>
                          {log.apiKeyName && <span className="truncate text-xs text-muted-foreground">Key {log.apiKeyName}</span>}
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex min-w-[150px] flex-col gap-1">
                          <span className="font-mono text-xs">{log.ipAddress || "-"}</span>
                          <Badge variant="outline">{log.source}</Badge>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex min-w-[170px] flex-col gap-1 text-xs">
                          <span>{log.method} {log.path}</span>
                          <span className="text-muted-foreground">
                            {log.model} / {log.size} / {qualityLabel(log.quality)}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className="tabular-nums">{formatDuration(log.durationMs)}</TableCell>
                      <TableCell className="tabular-nums">{log.creditsCost}</TableCell>
                      <TableCell>
                        {log.prompt ? (
                          <Button type="button" variant="ghost" size="icon-xs" aria-label="查看提示词" title="查看提示词" onClick={() => openUsageDetail(log, "prompt")}>
                            <Eye data-icon="inline-start" />
                          </Button>
                        ) : (
                          "-"
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="flex min-w-[140px] items-center gap-2">
                          <Badge variant={log.success ? "secondary" : "destructive"}>{log.success ? "成功" : "失败"}</Badge>
                          <span className="text-xs text-muted-foreground">{log.statusCode ?? "-"} / {log.status}</span>
                          <Button type="button" variant="ghost" size="icon-xs" aria-label="查看结果详情" title="查看结果详情" onClick={() => openUsageDetail(log, "result")}>
                            <Eye data-icon="inline-start" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                  {!usageLoading && usageLogs.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={8} className="py-8 text-center text-sm text-muted-foreground">
                        暂无用量日志
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>

              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="text-sm text-muted-foreground">
                  共 {usageTotal} 条，第 {usagePage} / {usageTotalPages} 页
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" disabled={!usageCanPrev || usageLoading} onClick={() => setUsageOffset((value) => Math.max(0, value - usageLimit))}>
                    上一页
                  </Button>
                  <Button variant="outline" disabled={!usageCanNext || usageLoading} onClick={() => setUsageOffset((value) => value + usageLimit)}>
                    下一页
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>

        </TabsContent>

        <TabsContent value="pricing">
          <Card>
            <CardHeader>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <CardTitle>价格规则</CardTitle>
                  <CardDescription>删除后规则会从计费和前台价格列表中移除。</CardDescription>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button variant="outline" onClick={openNewPricingDialog}>
                    <Plus data-icon="inline-start" />
                    新增定价
                  </Button>
                  <Button variant="outline" onClick={() => void toggle4K()}>
                    {platform.allow4k ? <ToggleRight data-icon="inline-start" /> : <ToggleLeft data-icon="inline-start" />}
                    {platform.allow4k ? "禁用 4K" : "启用 4K"}
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>模型</TableHead>
                    <TableHead>分辨率档位</TableHead>
                    <TableHead>质量</TableHead>
                    <TableHead>积分</TableHead>
                    <TableHead>操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {visiblePricing.map((rule) => (
                    <TableRow key={rule.id}>
                      <TableCell>{rule.model}</TableCell>
                      <TableCell>{rule.size.toUpperCase()}</TableCell>
                      <TableCell>{qualityLabel(rule.quality)}</TableCell>
                      <TableCell className="tabular-nums">{rule.credits}</TableCell>
                      <TableCell>
                        <div className="flex gap-2">
                          <Button variant="outline" size="icon" aria-label="编辑定价" onClick={() => editPricing(rule)}>
                            <Pencil />
                          </Button>
                          <Button variant="outline" size="icon" aria-label="删除定价" onClick={() => void deletePricing(rule)}>
                            <Trash2 />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="codes">
          <Card>
            <CardHeader>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <CardTitle>兑换码列表</CardTitle>
                  <CardDescription>勾选后可批量复制、停用或删除。</CardDescription>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button variant="outline" onClick={() => setCodeDialogOpen(true)}>
                    <Gift data-icon="inline-start" />
                    生成兑换码
                  </Button>
                  <Button variant="outline" disabled={selectedCodes.length === 0} onClick={copySelectedCodes}>
                    <Copy data-icon="inline-start" />
                    复制
                  </Button>
                  <Button variant="outline" disabled={selectedCodes.length === 0} onClick={() => void bulkCodes("disable")}>
                    <Ban data-icon="inline-start" />
                    停用
                  </Button>
                  <Button variant="outline" disabled={selectedCodes.length === 0} onClick={() => void bulkCodes("delete")}>
                    <Trash2 data-icon="inline-start" />
                    删除
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-10">
                      <input type="checkbox" className="size-4" checked={allCodesSelected} onChange={toggleAllCodes} aria-label="选择全部兑换码" />
                    </TableHead>
                    <TableHead>兑换码</TableHead>
                    <TableHead>额度</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>过期</TableHead>
                    <TableHead>使用者</TableHead>
                    <TableHead>使用时间</TableHead>
                    <TableHead>创建</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {codes.map((code) => (
                    <TableRow key={code.id}>
                      <TableCell>
                        <input
                          type="checkbox"
                          className="size-4"
                          checked={selectedCodeSet.has(code.id)}
                          onChange={() => toggleCode(code.id)}
                          aria-label={`选择兑换码 ${code.code}`}
                        />
                      </TableCell>
                      <TableCell className="font-mono text-xs">{code.code}</TableCell>
                      <TableCell className="tabular-nums">{code.credits}</TableCell>
                      <TableCell>
                        <Badge variant={code.status === "ACTIVE" ? "secondary" : code.status === "USED" ? "outline" : "destructive"}>{code.status}</Badge>
                      </TableCell>
                      <TableCell>{formatDate(code.expiresAt)}</TableCell>
                      <TableCell>{code.usedByEmail || "-"}</TableCell>
                      <TableCell>{formatDate(code.usedAt)}</TableCell>
                      <TableCell>{formatDate(code.createdAt)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="docs" className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <Card>
            <CardHeader>
              <CardTitle>文档编辑</CardTitle>
              <CardDescription>正文使用 Markdown，上传图片后会自动插入图片语法。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="flex flex-col gap-5" onSubmit={(event) => void saveContent("docs", event)}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="docs-title">标题</FieldLabel>
                    <Input id="docs-title" value={docsTitle} onChange={(event) => setDocsTitle(event.target.value)} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="docs-active">状态</FieldLabel>
                    <Select value={docsActive ? "true" : "false"} onValueChange={(value) => setDocsActive(value === "true")}>
                      <SelectTrigger id="docs-active" className="w-full">
                        <SelectValue>{docsActive ? "发布" : "隐藏"}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          <SelectItem value="true">发布</SelectItem>
                          <SelectItem value="false">隐藏</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="docs-image">图片</FieldLabel>
                    <Input id="docs-image" type="file" accept="image/png,image/jpeg,image/webp,image/gif" onChange={(event) => void uploadDocsImage(event)} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="docs-body">Markdown</FieldLabel>
                    <Textarea id="docs-body" className="min-h-[420px] font-mono text-sm" value={docsBody} onChange={(event) => setDocsBody(event.target.value)} />
                  </Field>
                </FieldGroup>
                <Button type="submit">
                  <Save data-icon="inline-start" />
                  保存文档
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{docsTitle || docs?.title || "文档预览"}</CardTitle>
              <CardDescription>更新于 {formatDate(docs?.updatedAt)}</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">{docsBody.trim() ? renderMarkdown(docsBody) : <div className="text-sm text-muted-foreground">暂无内容</div>}</CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="announcement" className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <Card>
            <CardHeader>
              <CardTitle>公告编辑</CardTitle>
              <CardDescription>启用后用户登录会弹窗提醒，也可从右上角按钮再次查看。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="flex flex-col gap-5" onSubmit={(event) => void saveContent("announcement", event)}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="announcement-title">标题</FieldLabel>
                    <Input id="announcement-title" value={announcementTitle} onChange={(event) => setAnnouncementTitle(event.target.value)} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="announcement-active">状态</FieldLabel>
                    <Select value={announcementActive ? "true" : "false"} onValueChange={(value) => setAnnouncementActive(value === "true")}>
                      <SelectTrigger id="announcement-active" className="w-full">
                        <SelectValue>{announcementActive ? "启用" : "停用"}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          <SelectItem value="true">启用</SelectItem>
                          <SelectItem value="false">停用</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="announcement-body">Markdown</FieldLabel>
                    <Textarea id="announcement-body" className="min-h-[420px] font-mono text-sm" value={announcementBody} onChange={(event) => setAnnouncementBody(event.target.value)} />
                  </Field>
                </FieldGroup>
                <Button type="submit">
                  <Bell data-icon="inline-start" />
                  保存公告
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{announcementTitle || announcement?.title || "公告预览"}</CardTitle>
              <CardDescription>更新于 {formatDate(announcement?.updatedAt)}</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              {announcementBody.trim() ? renderMarkdown(announcementBody) : <div className="text-sm text-muted-foreground">暂无内容</div>}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="openai">
          <Card>
            <CardHeader>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <CardTitle>API 节点</CardTitle>
                  <CardDescription>生成任务会按优先级轮询可调度节点，连续失败后自动熔断。</CardDescription>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button variant="outline" onClick={openNewOpenAIEndpointDialog}>
                    <Plus data-icon="inline-start" />
                    新增节点
                  </Button>
                  <Badge variant="secondary">{openAI?.endpoints?.filter((endpoint) => endpoint.enabled && endpoint.schedulable).length ?? 0} 个可调度</Badge>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>节点</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>优先级</TableHead>
                    <TableHead>失败</TableHead>
                    <TableHead>熔断</TableHead>
                    <TableHead>最近结果</TableHead>
                    <TableHead>操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(openAI?.endpoints ?? []).map((endpoint) => (
                    <TableRow key={endpoint.id}>
                      <TableCell>
                        <div className="flex min-w-[220px] flex-col gap-1">
                          <span className="font-medium">{endpoint.name}</span>
                          <span className="truncate text-xs text-muted-foreground">{endpoint.baseUrl}</span>
                          <div className="flex flex-wrap gap-1">
                            <Badge variant={endpoint.hasApiKey ? "secondary" : "outline"}>{endpoint.hasApiKey ? "密钥已配置" : "密钥未配置"}</Badge>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-wrap gap-1">
                          <Badge variant={endpoint.enabled ? "secondary" : "outline"}>{endpoint.enabled ? "启用" : "停用"}</Badge>
                          <Badge variant={endpoint.schedulable ? "secondary" : "outline"}>{endpoint.schedulable ? "可调度" : "仅保存"}</Badge>
                        </div>
                      </TableCell>
                      <TableCell className="tabular-nums">{endpoint.priority}</TableCell>
                      <TableCell className="tabular-nums">{endpoint.failureCount}</TableCell>
                      <TableCell>{endpoint.circuitOpenUntil ? formatDate(endpoint.circuitOpenUntil) : "-"}</TableCell>
                      <TableCell>
                        <div className="flex min-w-[170px] flex-col gap-1 text-xs text-muted-foreground">
                          <span>成功 {formatDate(endpoint.lastSuccessAt)}</span>
                          <span>失败 {formatDate(endpoint.lastFailureAt)}</span>
                          {endpoint.lastError && <span className="line-clamp-1 text-destructive">{endpoint.lastError}</span>}
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-wrap gap-2">
                          <Button variant="outline" size="icon" aria-label="编辑 API 节点" onClick={() => editOpenAIEndpoint(endpoint)}>
                            <Pencil />
                          </Button>
                          <Button variant="outline" size="icon" aria-label="恢复熔断" onClick={() => void resetOpenAIEndpoint(endpoint)}>
                            <RefreshCw />
                          </Button>
                          <Button variant="outline" size="icon" aria-label="删除 API 节点" onClick={() => void deleteOpenAIEndpoint(endpoint)}>
                            <Trash2 />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="payment">
          <Card className="max-w-2xl">
            <CardHeader>
              <CardTitle>易支付配置</CardTitle>
              <CardDescription>配置兼容易支付 submit.php/notify_url 的在线充值。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="flex flex-col gap-5" onSubmit={saveEPay}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="epay-gateway">网关地址</FieldLabel>
                    <Input id="epay-gateway" value={epayGateway} onChange={(event) => setEPayGateway(event.target.value)} />
                    <FieldDescription>填写到域名即可，系统会自动拼接 /submit.php。</FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="epay-pid">商户 ID</FieldLabel>
                    <Input id="epay-pid" value={epayPID} onChange={(event) => setEPayPID(event.target.value)} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="epay-key">商户密钥</FieldLabel>
                    <Input id="epay-key" type="password" value={epayKey} onChange={(event) => setEPayKey(event.target.value)} placeholder={epay?.hasKey ? "留空则保持当前密钥" : "输入商户密钥"} />
                    <FieldDescription>密钥只保存到后台配置，页面不会回显。</FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="epay-name">商品名称</FieldLabel>
                    <Input id="epay-name" value={epayName} onChange={(event) => setEPayName(event.target.value)} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="epay-rate">每 1 元到账积分</FieldLabel>
                    <Input id="epay-rate" type="number" min={1} value={epayCreditsPerYuan} onChange={(event) => setEPayCreditsPerYuan(Number(event.target.value))} />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="epay-enabled">启用状态</FieldLabel>
                    <Select value={epayEnabled ? "true" : "false"} onValueChange={(value) => setEPayEnabled(value === "true")}>
                      <SelectTrigger id="epay-enabled" className="w-full">
                        <SelectValue>{epayEnabled ? "启用" : "停用"}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          <SelectItem value="true">启用</SelectItem>
                          <SelectItem value="false">停用</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                </FieldGroup>
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant={epay?.enabled ? "secondary" : "outline"}>{epay?.enabled ? "支付已启用" : "支付未启用"}</Badge>
                  <Badge variant={epay?.hasKey ? "secondary" : "outline"}>{epay?.hasKey ? "密钥已配置" : "密钥未配置"}</Badge>
                </div>
                <Button type="submit">
                  <CreditCard data-icon="inline-start" />
                  保存配置
                </Button>
              </form>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <Dialog open={usageRetentionDialogOpen} onOpenChange={setUsageRetentionDialogOpen}>
        <DialogContent className="max-h-[90svh] overflow-auto sm:max-w-md">
          <DialogHeader>
            <DialogTitle>自动清理</DialogTitle>
            <DialogDescription>worker 每小时清理超过保留天数的用量日志，设置 0 表示关闭。</DialogDescription>
          </DialogHeader>
          <form id="usage-retention-form" className="flex flex-col gap-5" onSubmit={saveUsageRetention}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="usage-retention">保留天数</FieldLabel>
                <Input id="usage-retention" type="number" min={0} max={3650} value={usageRetentionDays} onChange={(event) => setUsageRetentionDays(Number(event.target.value))} />
              </Field>
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setUsageRetentionDialogOpen(false)}>
              取消
            </Button>
            <Button type="submit" form="usage-retention-form">
              <Save data-icon="inline-start" />
              保存策略
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={usageDeleteDialogOpen} onOpenChange={setUsageDeleteDialogOpen}>
        <DialogContent className="max-h-[90svh] overflow-auto sm:max-w-md">
          <DialogHeader>
            <DialogTitle>手动删除</DialogTitle>
            <DialogDescription>立即删除指定时间之前的日志，用于控制日志体积。</DialogDescription>
          </DialogHeader>
          <form id="usage-delete-form" className="flex flex-col gap-5" onSubmit={deleteUsageLogs}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="usage-delete-days">删除多少天前</FieldLabel>
                <Input id="usage-delete-days" type="number" min={1} max={3650} value={usageDeleteDays} onChange={(event) => setUsageDeleteDays(Number(event.target.value))} />
                <FieldDescription>未填写截止时间时，按这个天数删除。</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="usage-delete-before">指定截止时间</FieldLabel>
                <Input id="usage-delete-before" type="datetime-local" value={usageDeleteBefore} onChange={(event) => setUsageDeleteBefore(event.target.value)} />
              </Field>
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setUsageDeleteDialogOpen(false)}>
              取消
            </Button>
            <Button type="submit" variant="outline" form="usage-delete-form">
              <Trash2 data-icon="inline-start" />
              删除日志
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={usageDetailLog !== null} onOpenChange={(open) => (!open ? closeUsageDetail() : undefined)}>
        <DialogContent className="max-h-[90svh] overflow-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{usageDetailKind === "prompt" ? "提示词详情" : "结果详情"}</DialogTitle>
            <DialogDescription>
              {usageDetailLog ? `${usageDetailLog.userEmail || "-"}，${formatDate(usageDetailLog.createdAt)}` : "查看用量日志详情"}
            </DialogDescription>
          </DialogHeader>
          {usageDetailLog && usageDetailKind === "prompt" && (
            <div className="whitespace-pre-wrap break-words rounded-lg border bg-muted/30 p-3 text-sm leading-6">
              {usageDetailLog.prompt || "-"}
            </div>
          )}
          {usageDetailLog && usageDetailKind === "result" && (
            <div className="flex flex-col gap-4">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={usageDetailLog.success ? "secondary" : "destructive"}>{usageDetailLog.success ? "成功" : "失败"}</Badge>
                <Badge variant="outline">{usageDetailLog.statusCode ?? "-"} / {usageDetailLog.status}</Badge>
                <Badge variant="outline">{formatDuration(usageDetailLog.durationMs)}</Badge>
                <Badge variant="outline">{usageDetailLog.creditsCost} 积分</Badge>
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="flex flex-col gap-1 rounded-lg border bg-muted/30 p-3">
                  <span className="text-xs text-muted-foreground">调用记录</span>
                  <span className="break-all text-sm">{usageDetailLog.method} {usageDetailLog.path}</span>
                </div>
                <div className="flex flex-col gap-1 rounded-lg border bg-muted/30 p-3">
                  <span className="text-xs text-muted-foreground">模型 / 尺寸 / 质量</span>
                  <span className="break-words text-sm">
                    {usageDetailLog.model || "-"} / {usageDetailLog.size || "-"} / {qualityLabel(usageDetailLog.quality)}
                  </span>
                </div>
                <div className="flex flex-col gap-1 rounded-lg border bg-muted/30 p-3">
                  <span className="text-xs text-muted-foreground">IP / 来源</span>
                  <span className="break-all text-sm">{usageDetailLog.ipAddress || "-"} / {usageDetailLog.source}</span>
                </div>
                <div className="flex flex-col gap-1 rounded-lg border bg-muted/30 p-3">
                  <span className="text-xs text-muted-foreground">参考图数量 / 完成时间</span>
                  <span className="break-words text-sm">
                    {usageDetailLog.referenceCount} / {formatDate(usageDetailLog.completedAt)}
                  </span>
                </div>
              </div>
              <div className="flex flex-col gap-2">
                <span className="text-sm font-medium">失败原因</span>
                <div className="whitespace-pre-wrap break-words rounded-lg border bg-muted/30 p-3 text-sm leading-6">
                  {usageDetailLog.errorMessage || "无"}
                </div>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={closeUsageDetail}>
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={pricingDialogOpen} onOpenChange={(open) => (open ? setPricingDialogOpen(true) : closePricingDialog())}>
        <DialogContent className="max-h-[90svh] overflow-auto sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{editingPricingId ? "编辑定价" : "新增定价"}</DialogTitle>
            <DialogDescription>按模型、分辨率档位和质量维护每次调用积分。</DialogDescription>
          </DialogHeader>
          <form id="pricing-form" className="flex flex-col gap-5" onSubmit={savePricing}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="pricing-model">模型</FieldLabel>
                <Input id="pricing-model" value={model} onChange={(event) => setModel(event.target.value)} />
              </Field>
              <Field>
                <FieldLabel htmlFor="pricing-size">分辨率档位</FieldLabel>
                <Select value={size} onValueChange={(value) => setSize(value ?? size)}>
                  <SelectTrigger id="pricing-size" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {enabledResolutionBuckets.map((item) => (
                        <SelectItem key={item} value={item}>
                          {item.toUpperCase()}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor="pricing-quality">质量</FieldLabel>
                <Select value={quality} onValueChange={(value) => setQuality(value ?? quality)}>
                  <SelectTrigger id="pricing-quality" className="w-full">
                    <SelectValue>{qualityLabel(quality)}</SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {qualities.map((item) => (
                        <SelectItem key={item} value={item}>
                          {qualityLabel(item)}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor="credits">积分</FieldLabel>
                <Input id="credits" type="number" min={1} value={credits} onChange={(event) => setCredits(Number(event.target.value))} />
              </Field>
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={closePricingDialog}>
              取消
            </Button>
            <Button type="submit" form="pricing-form">
              {editingPricingId ? <Save data-icon="inline-start" /> : <Plus data-icon="inline-start" />}
              {editingPricingId ? "更新" : "新增"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={codeDialogOpen} onOpenChange={(open) => (open ? setCodeDialogOpen(true) : closeCodeDialog())}>
        <DialogContent className="max-h-[90svh] overflow-auto sm:max-w-md">
          <DialogHeader>
            <DialogTitle>生成兑换码</DialogTitle>
            <DialogDescription>生成后用户可在账单页兑换为余额。</DialogDescription>
          </DialogHeader>
          <form id="redemption-code-form" className="flex flex-col gap-5" onSubmit={generateCodes}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="code-credits">额度</FieldLabel>
                <Input id="code-credits" type="number" min={1} value={codeCredits} onChange={(event) => setCodeCredits(Number(event.target.value))} />
              </Field>
              <Field>
                <FieldLabel htmlFor="code-count">数量</FieldLabel>
                <Input id="code-count" type="number" min={1} max={500} value={codeCount} onChange={(event) => setCodeCount(Number(event.target.value))} />
              </Field>
              <Field>
                <FieldLabel htmlFor="code-expires">过期时间</FieldLabel>
                <Input id="code-expires" type="datetime-local" value={codeExpiresAt} onChange={(event) => setCodeExpiresAt(event.target.value)} />
                <FieldDescription>留空则长期有效。</FieldDescription>
              </Field>
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={closeCodeDialog}>
              取消
            </Button>
            <Button type="submit" form="redemption-code-form">
              <Gift data-icon="inline-start" />
              生成
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={openAIEndpointDialogOpen} onOpenChange={(open) => (open ? setOpenAIEndpointDialogOpen(true) : closeOpenAIEndpointDialog())}>
        <DialogContent className="max-h-[90svh] overflow-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{editingOpenAIEndpointId ? "编辑 API 节点" : "新增 API 节点"}</DialogTitle>
            <DialogDescription>配置兼容 OpenAI 图像接口的 Base URL、密钥和调度策略。</DialogDescription>
          </DialogHeader>
          <form id="openai-endpoint-form" className="flex flex-col gap-5" onSubmit={saveOpenAIEndpoint}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="openai-endpoint-name">名称</FieldLabel>
                <Input id="openai-endpoint-name" value={openAIEndpointName} onChange={(event) => setOpenAIEndpointName(event.target.value)} />
              </Field>
              <Field>
                <FieldLabel htmlFor="openai-base-url">Base URL</FieldLabel>
                <Input id="openai-base-url" value={openAIBaseUrl} onChange={(event) => setOpenAIBaseUrl(event.target.value)} />
              </Field>
              <Field>
                <FieldLabel htmlFor="openai-key">API Key</FieldLabel>
                <Input
                  id="openai-key"
                  type="password"
                  value={openAIKey}
                  onChange={(event) => setOpenAIKey(event.target.value)}
                  placeholder={editingOpenAIEndpointId ? "留空则保持当前密钥" : "输入 API Key"}
                />
                <FieldDescription>密钥只写入后台配置，保存后不会在页面回显。</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="openai-priority">优先级</FieldLabel>
                <Input id="openai-priority" type="number" min={1} max={10000} value={openAIPriority} onChange={(event) => setOpenAIPriority(Math.max(1, Number(event.target.value) || 100))} />
                <FieldDescription>数字越小越优先，同优先级按最近使用时间轮询。</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="openai-enabled">启用状态</FieldLabel>
                <Select value={openAIEnabled ? "true" : "false"} onValueChange={(value) => setOpenAIEnabled(value === "true")}>
                  <SelectTrigger id="openai-enabled" className="w-full">
                    <SelectValue>{openAIEnabled ? "启用" : "停用"}</SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="true">启用</SelectItem>
                      <SelectItem value="false">停用</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor="openai-schedulable">调度状态</FieldLabel>
                <Select value={openAISchedulable ? "true" : "false"} onValueChange={(value) => setOpenAISchedulable(value === "true")}>
                  <SelectTrigger id="openai-schedulable" className="w-full">
                    <SelectValue>{openAISchedulable ? "可调度" : "仅保存"}</SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="true">可调度</SelectItem>
                      <SelectItem value="false">仅保存</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
            </FieldGroup>
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={openAI?.hasOpenaiApiKey ? "secondary" : "outline"}>{openAI?.hasOpenaiApiKey ? "存在可用密钥" : "未配置密钥"}</Badge>
              {openAI?.usesEnvironmentKey && <Badge variant="outline">使用环境变量兜底</Badge>}
            </div>
          </form>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={closeOpenAIEndpointDialog}>
              取消
            </Button>
            <Button type="submit" form="openai-endpoint-form">
              {editingOpenAIEndpointId ? <Save data-icon="inline-start" /> : <Plus data-icon="inline-start" />}
              {editingOpenAIEndpointId ? "更新节点" : "新增节点"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function formatDuration(value?: number) {
  if (value == null) return "-"
  if (value < 1000) return `${value} ms`
  const seconds = value / 1000
  if (seconds < 60) return `${seconds.toFixed(1)} s`
  return `${Math.floor(seconds / 60)}m ${Math.round(seconds % 60)}s`
}

function AdminMetric({ icon: Icon, label, value }: { icon: LucideIcon; label: string; value: number }) {
  return (
    <Card>
      <CardContent className="flex items-center gap-3 p-4">
        <div className="flex size-10 items-center justify-center rounded-lg bg-muted">
          <Icon />
        </div>
        <div>
          <div className="text-sm text-muted-foreground">{label}</div>
          <div className="text-2xl font-semibold">{value}</div>
        </div>
      </CardContent>
    </Card>
  )
}
