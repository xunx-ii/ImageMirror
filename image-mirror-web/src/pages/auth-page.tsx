import { useState } from "react"
import type { FormEvent } from "react"
import { ImagePlus, LogIn, UserPlus } from "lucide-react"
import { Link, useNavigate } from "react-router-dom"
import { toast } from "sonner"

import { api, errorMessage } from "@/api/client"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useAuthStore } from "@/stores/auth"
import type { TokenPair, User } from "@/types"

type AuthPageProps = {
  mode: "login" | "register"
}

export function AuthPage({ mode }: AuthPageProps) {
  const navigate = useNavigate()
  const setSession = useAuthStore((state) => state.setSession)
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [loading, setLoading] = useState(false)
  const isLogin = mode === "login"

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setLoading(true)
    try {
      const { data } = await api.post<{ user: User; tokens: TokenPair }>(
        isLogin ? "/api/auth/login" : "/api/auth/register",
        { email, password }
      )
      setSession(data.user, data.tokens)
      toast.success(isLogin ? "登录成功" : "注册成功")
      navigate("/dashboard")
    } catch (error) {
      toast.error(errorMessage(error))
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="flex min-h-svh items-center justify-center bg-muted/30 p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <div className="mb-2 flex size-10 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <ImagePlus />
          </div>
          <CardTitle>{isLogin ? "登录 ImageMirror" : "创建账户"}</CardTitle>
          <CardDescription>
            {isLogin ? "进入图像生成中转工作台" : "注册后可充值积分并创建 API Key"}
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
            </FieldGroup>
            <Button disabled={loading} type="submit">
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
  )
}
