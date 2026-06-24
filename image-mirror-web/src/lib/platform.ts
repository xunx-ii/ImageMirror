import type { PlatformSettings } from "@/types"

export const platformSettingsUpdatedEvent = "image-mirror-platform-settings-updated"

export const defaultPlatformSettings: PlatformSettings = {
  maxResolutionBucket: "4k",
  allow4k: true,
  siteTitle: "IM",
  siteSubtitle: "AI图像生成平台",
  loadingText: "IM AI图像生成平台",
  apiKeysEnabled: true,
}

export function mergePlatformSettings(settings?: Partial<PlatformSettings> | null): PlatformSettings {
  const maxResolutionBucket = settings?.maxResolutionBucket === "2k" ? "2k" : "4k"
  const siteTitle = settings?.siteTitle?.trim() || defaultPlatformSettings.siteTitle
  const siteSubtitle = settings?.siteSubtitle?.trim() || defaultPlatformSettings.siteSubtitle
  const loadingText = settings?.loadingText?.trim() || defaultPlatformSettings.loadingText
  return {
    maxResolutionBucket,
    allow4k: maxResolutionBucket === "4k",
    siteTitle,
    siteSubtitle,
    loadingText,
    apiKeysEnabled: settings?.apiKeysEnabled ?? defaultPlatformSettings.apiKeysEnabled,
  }
}

export function platformDocumentTitle(settings: Pick<PlatformSettings, "siteTitle" | "siteSubtitle">) {
  const siteTitle = settings.siteTitle.trim() || defaultPlatformSettings.siteTitle
  const siteSubtitle = settings.siteSubtitle.trim()
  return siteSubtitle ? `${siteTitle} ${siteSubtitle}` : siteTitle
}

export function platformLoadingTitle(settings: Pick<PlatformSettings, "loadingText" | "siteTitle" | "siteSubtitle">) {
  return settings.loadingText.trim() || platformDocumentTitle(settings)
}

export function emitPlatformSettingsUpdated(settings: PlatformSettings) {
  window.dispatchEvent(new CustomEvent<PlatformSettings>(platformSettingsUpdatedEvent, { detail: settings }))
}
