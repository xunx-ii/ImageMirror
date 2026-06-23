export type User = {
  id: string
  email: string
  role: "USER" | "ADMIN"
  status: string
  balance: number
  lastLoginAt?: string
  createdAt: string
  updatedAt: string
}

export type TokenPair = {
  accessToken: string
  refreshToken: string
  expiresAt: string
}

export type ImageGeneration = {
  id: string
  userId: string
  apiKeyId?: string
  model: string
  prompt: string
  size: string
  quality: string
  status: "PENDING" | "PROCESSING" | "COMPLETED" | "FAILED" | "EXPIRED"
  storageUrl?: string
  referenceCount: number
  creditsCost: number
  errorMessage?: string
  expiresAt: string
  deletedAt?: string
  createdAt: string
  updatedAt: string
}

export type PricingRule = {
  id: string
  model: string
  size: string
  quality: string
  credits: number
  isActive: boolean
  createdAt: string
  updatedAt: string
}

export type ApiKey = {
  id: string
  userId: string
  name: string
  keyPrefix: string
  status: "ACTIVE"
  lastUsedAt?: string
  createdAt: string
}

export type CreatedApiKey = ApiKey & {
  plaintext: string
}

export type CreditTransaction = {
  id: string
  userId: string
  type: "RECHARGE" | "CONSUME" | "REFUND" | "ADMIN_ADJUST"
  amount: number
  balanceAfter: number
  description: string
  relatedId?: string
  createdAt: string
}

export type AdminOverview = {
  users: number
  images: number
  completed: number
  creditsConsumed: number
}

export type OpenAISettings = {
  openaiBaseUrl: string
  hasOpenaiApiKey: boolean
  usesEnvironmentKey: boolean
}

export type EPaySettings = {
  gateway: string
  pid: string
  hasKey: boolean
  name: string
  creditsPerYuan: number
  enabled: boolean
}

export type PaymentOrder = {
  id: string
  userId: string
  provider: string
  outTradeNo: string
  providerTradeNo?: string
  name: string
  amountCents: number
  credits: number
  status: "PENDING" | "PAID" | "FAILED"
  paidAt?: string
  createdAt: string
  updatedAt: string
}

export type RedemptionCode = {
  id: string
  code: string
  credits: number
  status: "ACTIVE" | "USED" | "DISABLED" | "EXPIRED"
  expiresAt?: string
  usedBy?: string
  usedByEmail?: string
  usedAt?: string
  createdBy?: string
  createdByEmail?: string
  createdAt: string
  updatedAt: string
}

export type RedemptionHistoryItem = {
  code: string
  credits: number
  redeemedAt: string
}

export type SiteContent = {
  key: "docs" | "announcement"
  title: string
  body: string
  isActive: boolean
  updatedAt: string
}

export type ContentAsset = {
  id: string
  kind: string
  filename: string
  url: string
  createdAt: string
}
