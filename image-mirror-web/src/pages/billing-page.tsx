import { useCallback, useEffect, useState } from "react"
import type { FormEvent } from "react"
import { BadgeCent, RefreshCw } from "lucide-react"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { formatDate } from "@/lib/format"
import { useAuthStore } from "@/stores/auth"
import type { CreditTransaction, PaymentOrder } from "@/types"

export function BillingPage() {
  const user = useAuthStore((state) => state.user)
  const refreshMe = useAuthStore((state) => state.refreshMe)
  const [amount, setAmount] = useState(100)
  const [transactions, setTransactions] = useState<CreditTransaction[]>([])
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    try {
      const { data } = await api.get<{ data: CreditTransaction[] }>("/api/billing/transactions?limit=100")
      setTransactions(data.data ?? [])
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

  return (
    <>
      <PageHeader
        title="账单"
        description="预付积分会在提交生成任务时扣减，失败任务自动退款。"
        action={
          <Button variant="outline" onClick={load}>
            <RefreshCw data-icon="inline-start" />
            刷新
          </Button>
        }
      />
      <div className="grid gap-4 lg:grid-cols-[340px_1fr]">
        <Card>
          <CardHeader>
            <CardTitle>在线充值</CardTitle>
            <CardDescription>当前余额 {user?.balance ?? 0} credits</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="flex flex-col gap-5" onSubmit={createPayment}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="amount">充值金额</FieldLabel>
                  <Input
                    id="amount"
                    type="number"
                    min={1}
                    value={amount}
                    onChange={(event) => setAmount(Number(event.target.value))}
                    required
                  />
                  <FieldDescription>提交后会跳转到易支付，到账积分按后台配置兑换。</FieldDescription>
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
    </>
  )
}
