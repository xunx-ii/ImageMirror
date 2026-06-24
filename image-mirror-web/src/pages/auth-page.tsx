import { useEffect, useState } from "react"
import type { FormEvent } from "react"
import { ImagePlus, LogIn, UserPlus } from "lucide-react"
import { Link, useNavigate } from "react-router-dom"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldContent, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { renderMarkdown } from "@/lib/markdown"
import { defaultPlatformSettings, mergePlatformSettings, platformDocumentTitle } from "@/lib/platform"
import { useAuthStore } from "@/stores/auth"
import type { PlatformSettings, SiteContent, TokenPair, User } from "@/types"

type AuthPageProps = {
  mode: "login" | "register"
}

type LegalContentKey = "terms" | "privacy"

const fallbackLegalContent: Record<LegalContentKey, SiteContent> = {
  terms: {
    key: "terms",
    title: "服务条款",
    body: "# 服务条款\n\n暂无内容。",
    isActive: true,
    updatedAt: "",
  },
  privacy: {
    key: "privacy",
    title: "隐私政策",
    body: "# 隐私政策\n\n暂无内容。",
    isActive: true,
    updatedAt: "",
  },
}

async function loadLegalContent(key: LegalContentKey) {
  try {
    const { data } = await api.get<SiteContent>(`/api/content/${key}`)
    return data
  } catch {
    return fallbackLegalContent[key]
  }
}

export function AuthPage({ mode }: AuthPageProps) {
  const navigate = useNavigate()
  const setSession = useAuthStore((state) => state.setSession)
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [loading, setLoading] = useState(false)
  const [platform, setPlatform] = useState<PlatformSettings>(defaultPlatformSettings)
  const [legalAcceptedByMode, setLegalAcceptedByMode] = useState<Record<AuthPageProps["mode"], boolean>>({
    login: false,
    register: false,
  })
  const [legalDialog, setLegalDialog] = useState<LegalContentKey | null>(null)
  const [termsContent, setTermsContent] = useState<SiteContent>(fallbackLegalContent.terms)
  const [privacyContent, setPrivacyContent] = useState<SiteContent>(fallbackLegalContent.privacy)
  const isLogin = mode === "login"
  const legalAccepted = legalAcceptedByMode[mode]
  const activeLegalContent = legalDialog === "terms" ? termsContent : legalDialog === "privacy" ? privacyContent : null

  useEffect(() => {
    let active = true
    document.title = platformDocumentTitle(defaultPlatformSettings)
    api
      .get<PlatformSettings>("/api/settings/platform")
      .then((response) => {
        if (!active) return
        const settings = mergePlatformSettings(response.data)
        setPlatform(settings)
        document.title = platformDocumentTitle(settings)
      })
      .catch(() => {
        document.title = platformDocumentTitle(defaultPlatformSettings)
      })
    void Promise.all([loadLegalContent("terms"), loadLegalContent("privacy")]).then(([terms, privacy]) => {
      if (!active) return
      setTermsContent(terms)
      setPrivacyContent(privacy)
    })
    return () => {
      active = false
    }
  }, [])

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!legalAccepted) {
      toast.error("请先阅读并同意服务条款和隐私政策")
      return
    }
    setLoading(true)
    try {
      const { data } = await api.post<{ user: User; tokens: TokenPair }>(
        isLogin ? "/api/auth/login" : "/api/auth/register",
        { email, password }
      )
      setSession(data.user, data.tokens)
      toast.success(isLogin ? "登录成功" : "注册成功")
      navigate("/generate")
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setLoading(false)
    }
  }

  return (
    <>
      <main className="flex min-h-svh items-center justify-center bg-muted/30 p-4">
        <Card className="w-full max-w-md">
          <CardHeader>
            <div className="mb-2 flex size-10 items-center justify-center rounded-lg bg-primary text-primary-foreground">
              <ImagePlus />
            </div>
            <CardTitle>{isLogin ? `登录 ${platform.siteTitle}` : "创建账户"}</CardTitle>
            <CardDescription>
              {isLogin ? platform.siteSubtitle : "注册后可充值积分并创建 API Key"}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form className="flex flex-col gap-5" onSubmit={submit}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="email">邮箱</FieldLabel>
                  <Input
                    id="email"
                    type="email"
                    value={email}
                    autoComplete="email"
                    onChange={(event) => setEmail(event.target.value)}
                    required
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="password">密码</FieldLabel>
                  <Input
                    id="password"
                    type="password"
                    value={password}
                    minLength={8}
                    autoComplete={isLogin ? "current-password" : "new-password"}
                    onChange={(event) => setPassword(event.target.value)}
                    required
                  />
                </Field>
                <Field orientation="horizontal">
                  <Checkbox id="legal-accepted" checked={legalAccepted} onCheckedChange={(checked) => setLegalAcceptedByMode((state) => ({ ...state, [mode]: checked }))} />
                  <FieldContent>
                    <div className="flex flex-wrap items-center gap-1 text-sm leading-snug">
                      <FieldLabel htmlFor="legal-accepted" className="font-normal">
                        我已阅读并同意
                      </FieldLabel>
                      <button type="button" className="font-medium text-foreground underline underline-offset-4" onClick={() => setLegalDialog("terms")}>
                        服务条款
                      </button>
                      <span className="text-muted-foreground">和</span>
                      <button type="button" className="font-medium text-foreground underline underline-offset-4" onClick={() => setLegalDialog("privacy")}>
                        隐私政策
                      </button>
                    </div>
                  </FieldContent>
                </Field>
              </FieldGroup>
              <Button disabled={loading || !legalAccepted} type="submit">
                {isLogin ? <LogIn data-icon="inline-start" /> : <UserPlus data-icon="inline-start" />}
                {loading ? "处理中" : isLogin ? "登录" : "注册"}
              </Button>
              <div className="text-center text-sm text-muted-foreground">
                {isLogin ? "还没有账户？" : "已经有账户？"}
                <Link className="ml-1 font-medium text-foreground underline underline-offset-4" to={isLogin ? "/register" : "/login"}>
                  {isLogin ? "注册" : "登录"}
                </Link>
              </div>
            </form>
          </CardContent>
        </Card>
      </main>

      <Dialog open={legalDialog !== null} onOpenChange={(open) => !open && setLegalDialog(null)}>
        <DialogContent className="max-h-[85svh] overflow-hidden sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{activeLegalContent?.title || "协议内容"}</DialogTitle>
            <DialogDescription>请阅读以下内容后继续。</DialogDescription>
          </DialogHeader>
          <div className="max-h-[60svh] overflow-auto rounded-lg border p-3 text-sm">
            {activeLegalContent?.body.trim() ? renderMarkdown(activeLegalContent.body) : <div className="text-sm text-muted-foreground">暂无内容</div>}
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
