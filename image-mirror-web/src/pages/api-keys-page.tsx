import { useCallback, useEffect, useState } from "react"
import type { FormEvent } from "react"
import { Copy, KeyRound, Plus, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { PageHeader } from "@/components/page-header"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { formatDate } from "@/lib/format"
import type { ApiKey, CreatedApiKey } from "@/types"

export function ApiKeysPage() {
  const [keys, setKeys] = useState<ApiKey[]>([])
  const [name, setName] = useState("default")
  const [created, setCreated] = useState<CreatedApiKey | null>(null)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    try {
      const { data } = await api.get<{ data: ApiKey[] }>("/api/api-keys")
      setKeys(data.data ?? [])
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

  async function createKey(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setLoading(true)
    try {
      const { data } = await api.post<CreatedApiKey>("/api/api-keys", { name })
      setCreated(data)
      toast.success("API Key 已创建")
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setLoading(false)
    }
  }

  async function deleteKey(id: string) {
    try {
      await api.delete(`/api/api-keys/${id}`)
      toast.success("API Key 已删除")
      await load()
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  return (
    <>
      <PageHeader title="API Key" description="开发者 API 使用 Bearer Key 鉴权，明文仅在创建时展示一次。" />
      <div className="grid gap-4 lg:grid-cols-[340px_1fr]">
        <Card>
          <CardHeader>
            <CardTitle>创建 Key</CardTitle>
            <CardDescription>Key 会以 SHA-256 哈希保存，列表只展示前缀，删除后不再保留。</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="flex flex-col gap-5" onSubmit={createKey}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="key-name">名称</FieldLabel>
                  <Input id="key-name" value={name} onChange={(event) => setName(event.target.value)} required />
                </Field>
              </FieldGroup>
              <Button disabled={loading} type="submit">
                <Plus data-icon="inline-start" />
                {loading ? "创建中" : "创建"}
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Key 列表</CardTitle>
          </CardHeader>
          <CardContent>
            {keys.length === 0 ? (
              <Empty>
                <EmptyHeader>
                  <EmptyMedia variant="icon">
                    <KeyRound />
                  </EmptyMedia>
                  <EmptyTitle>暂无 API Key</EmptyTitle>
                  <EmptyDescription>创建后可调用 `/v1/images/generations`。</EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>名称</TableHead>
                    <TableHead>前缀</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>最后使用</TableHead>
                    <TableHead className="w-20">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {keys.map((key) => (
                    <TableRow key={key.id}>
                      <TableCell>{key.name}</TableCell>
                      <TableCell className="font-mono text-xs">{key.keyPrefix}...</TableCell>
                      <TableCell>
                        <Badge variant={key.status === "ACTIVE" ? "secondary" : "outline"}>{key.status}</Badge>
                      </TableCell>
                      <TableCell>{formatDate(key.lastUsedAt)}</TableCell>
                      <TableCell>
                        <Button variant="destructive" size="icon-sm" aria-label="删除 API Key" onClick={() => void deleteKey(key.id)} disabled={key.status !== "ACTIVE"}>
                          <Trash2 />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>

      <Dialog open={!!created} onOpenChange={(open) => !open && setCreated(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>保存 API Key</DialogTitle>
            <DialogDescription>关闭后将无法再次查看明文。</DialogDescription>
          </DialogHeader>
          <Alert>
            <KeyRound />
            <AlertTitle>明文 Key</AlertTitle>
            <AlertDescription className="break-all font-mono text-xs">{created?.plaintext}</AlertDescription>
          </Alert>
          <Button
            variant="outline"
            onClick={() => {
              if (created?.plaintext) {
                void navigator.clipboard.writeText(created.plaintext)
                toast.success("已复制")
              }
            }}
          >
            <Copy data-icon="inline-start" />
            复制
          </Button>
        </DialogContent>
      </Dialog>
    </>
  )
}
