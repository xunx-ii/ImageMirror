import { useEffect, useState } from "react"
import { FileText } from "lucide-react"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { PageHeader } from "@/components/page-header"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { renderMarkdown } from "@/lib/markdown"
import type { SiteContent } from "@/types"

export function DocsPage() {
  const [docs, setDocs] = useState<SiteContent | null>(null)

  useEffect(() => {
    api
      .get<SiteContent>("/api/content/docs")
      .then((response) => setDocs(response.data))
      .catch((error) => toast.error(errorMessage(error)))
  }, [])

  return (
    <>
      <PageHeader title="文档" />
      <Card>
        <CardHeader>
          <CardTitle>{docs?.title || "文档"}</CardTitle>
        </CardHeader>
        <CardContent>
          {!docs?.body ? (
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <FileText />
                </EmptyMedia>
                <EmptyTitle>暂无文档</EmptyTitle>
                <EmptyDescription>管理员可以在管理页面维护 Markdown 文档。</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <div className="flex flex-col gap-3 text-sm">{renderMarkdown(docs.body)}</div>
          )}
        </CardContent>
      </Card>
    </>
  )
}
