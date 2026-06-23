import { useCallback, useEffect, useState } from "react"
import type { FormEvent } from "react"
import { BadgeCent, CreditCard, ImagePlus, Pencil, Plus, RefreshCw, Save, Settings, Shield, Trash2, Users } from "lucide-react"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { formatDate } from "@/lib/format"
import { resolutionBuckets } from "@/lib/image-size"
import type { AdminOverview, EPaySettings, OpenAISettings, PricingRule, User } from "@/types"

const qualities = ["low", "medium", "high", "auto"]

export function AdminPage() {
  const [overview, setOverview] = useState<AdminOverview | null>(null)
  const [users, setUsers] = useState<User[]>([])
  const [pricing, setPricing] = useState<PricingRule[]>([])
  const [openAI, setOpenAI] = useState<OpenAISettings | null>(null)
  const [epay, setEPay] = useState<EPaySettings | null>(null)
  const [model, setModel] = useState("gpt-image-2")
  const [size, setSize] = useState("1k")
  const [quality, setQuality] = useState("medium")
  const [credits, setCredits] = useState(8)
  const [editingPricingId, setEditingPricingId] = useState<string | null>(null)
  const [openAIBaseUrl, setOpenAIBaseUrl] = useState("https://api.openai.com")
  const [openAIKey, setOpenAIKey] = useState("")
  const [epayGateway, setEPayGateway] = useState("https://pay.example.com")
  const [epayPID, setEPayPID] = useState("")
  const [epayKey, setEPayKey] = useState("")
  const [epayName, setEPayName] = useState("ImageMirror credits")
  const [epayCreditsPerYuan, setEPayCreditsPerYuan] = useState(100)
  const [epayEnabled, setEPayEnabled] = useState(false)
  const [adjustments, setAdjustments] = useState<Record<string, number>>({})

  const load = useCallback(async () => {
    try {
      const [statsResponse, usersResponse, pricingResponse, openAIResponse, epayResponse] = await Promise.all([
        api.get<AdminOverview>("/api/admin/stats/overview"),
        api.get<{ data: User[] }>("/api/admin/users?limit=100"),
        api.get<{ data: PricingRule[] }>("/api/admin/pricing"),
        api.get<OpenAISettings>("/api/admin/config/openai"),
        api.get<EPaySettings>("/api/admin/config/epay"),
      ])
      setOverview(statsResponse.data)
      setUsers(usersResponse.data.data ?? [])
      setPricing(pricingResponse.data.data ?? [])
      setOpenAI(openAIResponse.data)
      setOpenAIBaseUrl(openAIResponse.data.openaiBaseUrl || "https://api.openai.com")
      setEPay(epayResponse.data)
      setEPayGateway(epayResponse.data.gateway || "https://pay.example.com")
      setEPayPID(epayResponse.data.pid ?? "")
      setEPayName(epayResponse.data.name || "ImageMirror credits")
      setEPayCreditsPerYuan(epayResponse.data.creditsPerYuan || 100)
      setEPayEnabled(epayResponse.data.enabled)
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void load()
    }, 0)
    return () => window.clearTimeout(timer)
  }, [load])

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

  async function saveOpenAI(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    try {
      const payload: { openaiBaseUrl: string; openaiApiKey?: string } = {
        openaiBaseUrl: openAIBaseUrl.trim(),
      }
      if (openAIKey.trim()) {
        payload.openaiApiKey = openAIKey.trim()
      }
      const { data } = await api.put<OpenAISettings>("/api/admin/config/openai", payload)
      setOpenAI(data)
      setOpenAIBaseUrl(data.openaiBaseUrl)
      setOpenAIKey("")
      toast.success("OpenAI 配置已保存")
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

  function editPricing(rule: PricingRule) {
    setEditingPricingId(rule.id)
    setModel(rule.model)
    setSize(rule.size)
    setQuality(rule.quality)
    setCredits(rule.credits)
  }

  function resetPricingForm() {
    setEditingPricingId(null)
    setModel("gpt-image-2")
    setSize("1k")
    setQuality("medium")
    setCredits(8)
  }

  return (
    <>
      <PageHeader
        title="管理"
        description="平台配置、定价规则、用户余额和运行概览。"
        action={
          <Button variant="outline" onClick={load}>
            <RefreshCw data-icon="inline-start" />
            刷新
          </Button>
        }
      />

      <Tabs defaultValue="overview" className="gap-4">
        <TabsList className="w-full">
          <TabsTrigger value="overview">概览</TabsTrigger>
          <TabsTrigger value="users">用户</TabsTrigger>
          <TabsTrigger value="pricing">定价</TabsTrigger>
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
              <Badge variant="secondary">用户 {users.length}</Badge>
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
                    <TableHead>余额</TableHead>
                    <TableHead>注册时间</TableHead>
                    <TableHead>调整</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {users.map((user) => (
                    <TableRow key={user.id}>
                      <TableCell>{user.email}</TableCell>
                      <TableCell>
                        <Badge variant={user.role === "ADMIN" ? "secondary" : "outline"}>{user.role}</Badge>
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
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="pricing" className="grid gap-4 lg:grid-cols-[360px_1fr]">
          <Card>
            <CardHeader>
              <CardTitle>{editingPricingId ? "编辑定价" : "新增定价"}</CardTitle>
              <CardDescription>按模型、分辨率档位和质量维护每次调用积分。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="flex flex-col gap-5" onSubmit={savePricing}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="pricing-model">模型</FieldLabel>
                    <Input id="pricing-model" value={model} onChange={(event) => setModel(event.target.value)} />
                  </Field>
                  <Field>
                    <FieldLabel>分辨率档位</FieldLabel>
                    <Select value={size} onValueChange={(value) => setSize(value ?? size)}>
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {resolutionBuckets.map((item) => (
                            <SelectItem key={item} value={item}>
                              {item.toUpperCase()}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field>
                    <FieldLabel>质量</FieldLabel>
                    <Select value={quality} onValueChange={(value) => setQuality(value ?? quality)}>
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {qualities.map((item) => (
                            <SelectItem key={item} value={item}>
                              {item}
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
                <div className="flex gap-2">
                  <Button type="submit">
                    {editingPricingId ? <Save data-icon="inline-start" /> : <Plus data-icon="inline-start" />}
                    {editingPricingId ? "更新" : "新增"}
                  </Button>
                  {editingPricingId && (
                    <Button type="button" variant="outline" onClick={resetPricingForm}>
                      取消
                    </Button>
                  )}
                </div>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>价格规则</CardTitle>
              <CardDescription>删除后规则会从计费和前台价格列表中移除。</CardDescription>
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
                  {pricing.map((rule) => (
                    <TableRow key={rule.id}>
                      <TableCell>{rule.model}</TableCell>
                      <TableCell>{rule.size.toUpperCase()}</TableCell>
                      <TableCell>{rule.quality}</TableCell>
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

        <TabsContent value="openai">
          <Card className="max-w-2xl">
            <CardHeader>
              <CardTitle>OpenAI 配置</CardTitle>
              <CardDescription>配置平台调用图像 API 使用的 Base URL 和 API Key。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="flex flex-col gap-5" onSubmit={saveOpenAI}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="openai-base-url">Base URL</FieldLabel>
                    <Input id="openai-base-url" value={openAIBaseUrl} onChange={(event) => setOpenAIBaseUrl(event.target.value)} />
                    <FieldDescription>默认使用 https://api.openai.com，也可以填写兼容 OpenAI 的中转地址。</FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="openai-key">API Key</FieldLabel>
                    <Input id="openai-key" type="password" value={openAIKey} onChange={(event) => setOpenAIKey(event.target.value)} placeholder={openAI?.hasOpenaiApiKey ? "留空则保持当前密钥" : "输入 API Key"} />
                    <FieldDescription>密钥只写入后台配置，保存后不会在页面回显。</FieldDescription>
                  </Field>
                </FieldGroup>
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant={openAI?.hasOpenaiApiKey ? "secondary" : "outline"}>{openAI?.hasOpenaiApiKey ? "API Key 已配置" : "API Key 未配置"}</Badge>
                  {openAI?.usesEnvironmentKey && <Badge variant="outline">使用环境变量兜底</Badge>}
                </div>
                <Button type="submit">
                  <Settings data-icon="inline-start" />
                  保存配置
                </Button>
              </form>
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
                        <SelectValue />
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
    </>
  )
}

function AdminMetric({ icon: Icon, label, value }: { icon: typeof Users; label: string; value: number }) {
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
