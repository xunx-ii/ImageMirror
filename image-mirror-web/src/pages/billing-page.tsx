import { useCallback, useEffect, useState } from "react"
import type { FormEvent } from "react"
import { BadgeCent, Gift, RefreshCw } from "lucide-react"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { formatDate } from "@/lib/format"
import { useAuthStore } from "@/stores/auth"
import type { CreditTransaction, PaymentOrder, RedemptionHistoryItem } from "@/types"

export function BillingPage() {
  const user = useAuthStore((state) => state.user)
  const refreshMe = useAuthStore((state) => state.refreshMe)
  const [amount, setAmount] = useState(100)
  const [redeemCode, setRedeemCode] = useState("")
  const [transactions, setTransactions] = useState<CreditTransaction[]>([])
  const [redemptions, setRedemptions] = useState<RedemptionHistoryItem[]>([])
  const [redemptionOpen, setRedemptionOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [redeeming, setRedeeming] = useState(false)

  const load = useCallback(async () => {
    try {
      const [txResponse, redemptionResponse] = await Promise.all([
        api.get<{ data: CreditTransaction[] }>("/api/billing/transactions?limit=100"),
        api.get<{ data: RedemptionHistoryItem[] }>("/api/billing/redemptions?limit=100"),
      ])
      setTransactions(txResponse.data.data ?? [])
      setRedemptions(redemptionResponse.data.data ?? [])
      await refreshMe()
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }, [refreshMe])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void load()
    }, 0)
    return () => window.clearTimeout(timer)
  }, [load])

  async function createPayment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setLoading(true)
    try {
      const { data } = await api.post<{ order: PaymentOrder; payUrl: string }>("/api/billing/epay/orders", {
        amount,
        payType: "alipay",
      })
      toast.success(`订单已创建，可到账 ${data.order.credits} credits`)
      window.open(data.payUrl, "_blank", "noopener,noreferrer")
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setLoading(false)
    }
  }

  async function redeem(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setRedeeming(true)
    try {
      const { data } = await api.post<{ code: { credits: number } }>("/api/billing/redeem", { code: redeemCode })
      toast.success(`兑换成功，到账 ${data.code.credits} credits`)
      setRedeemCode("")
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setRedeeming(false)
    }
  }

  return (
    <>
      <PageHeader
        title="账单"
        action={
          <Button variant="outline" onClick={load}>
            <RefreshCw data-icon="inline-start" />
            刷新
          </Button>
        }
      />
      <div className="grid gap-4 xl:grid-cols-[340px_1fr]">
        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle>在线充值</CardTitle>
              <CardDescription>当前余额 {user?.balance ?? 0} credits</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="flex flex-col gap-5" onSubmit={createPayment}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="amount">充值金额（人民币）</FieldLabel>
                    <Input
                      id="amount"
                      type="number"
                      min={1}
                      value={amount}
                      onChange={(event) => setAmount(Math.max(1, Math.floor(Number(event.target.value) || 1)))}
                      required
                    />
                    <FieldDescription>单位：人民币元。提交后会跳转到易支付，到账积分按后台配置兑换。</FieldDescription>
                  </Field>
                </FieldGroup>
                <Button disabled={loading} type="submit">
                  <BadgeCent data-icon="inline-start" />
                  {loading ? "创建订单中" : "去支付"}
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <div className="flex items-center justify-between gap-3">
                <div>
                  <CardTitle>兑换码</CardTitle>
                  <CardDescription>当前余额 {user?.balance ?? 0} credits</CardDescription>
                </div>
                <Button variant="outline" onClick={() => setRedemptionOpen(true)}>
                  <Gift data-icon="inline-start" />
                  兑换记录
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              <form className="flex flex-col gap-5" onSubmit={redeem}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="redeem-code">兑换码</FieldLabel>
                    <Input id="redeem-code" value={redeemCode} onChange={(event) => setRedeemCode(event.target.value.toUpperCase())} required />
                    <FieldDescription>输入管理员发放的兑换码后立即到账。</FieldDescription>
                  </Field>
                </FieldGroup>
                <Button disabled={redeeming || !redeemCode.trim()} type="submit">
                  <Gift data-icon="inline-start" />
                  {redeeming ? "兑换中" : "兑换"}
                </Button>
              </form>
            </CardContent>
          </Card>
        </div>

        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle>交易记录</CardTitle>
            </CardHeader>
            <CardContent>
              {transactions.length === 0 ? (
                <Empty>
                  <EmptyHeader>
                    <EmptyMedia variant="icon">
                      <BadgeCent />
                    </EmptyMedia>
                    <EmptyTitle>暂无交易</EmptyTitle>
                    <EmptyDescription>充值、扣费和退款会记录在这里。</EmptyDescription>
                  </EmptyHeader>
                </Empty>
              ) : (
                <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>类型</TableHead>
                    <TableHead>金额</TableHead>
                    <TableHead>余额</TableHead>
                    <TableHead>说明</TableHead>
                    <TableHead>时间</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {transactions.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell>
                        <Badge variant={item.amount > 0 ? "secondary" : "outline"}>{item.type}</Badge>
                      </TableCell>
                      <TableCell className="tabular-nums">{item.amount}</TableCell>
                      <TableCell className="tabular-nums">{item.balanceAfter}</TableCell>
                      <TableCell className="max-w-[260px] truncate">{item.description}</TableCell>
                      <TableCell>{formatDate(item.createdAt)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      <Dialog open={redemptionOpen} onOpenChange={setRedemptionOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>兑换记录</DialogTitle>
          </DialogHeader>
          {redemptions.length === 0 ? (
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <Gift />
                </EmptyMedia>
                <EmptyTitle>暂无兑换</EmptyTitle>
                <EmptyDescription>兑换码使用记录会显示在这里。</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <div className="max-h-[60vh] overflow-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>兑换码</TableHead>
                    <TableHead>积分</TableHead>
                    <TableHead>时间</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {redemptions.map((item) => (
                    <TableRow key={`${item.code}-${item.redeemedAt}`}>
                      <TableCell className="font-mono text-xs">{item.code}</TableCell>
                      <TableCell className="tabular-nums">{item.credits}</TableCell>
                      <TableCell>{formatDate(item.redeemedAt)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}
