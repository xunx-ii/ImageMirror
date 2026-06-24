import type { SiteContent } from "@/types"

export const siteContentUpdatedEvent = "image-mirror-site-content-updated"

export function emitSiteContentUpdated(content: SiteContent) {
  window.dispatchEvent(new CustomEvent<SiteContent>(siteContentUpdatedEvent, { detail: content }))
}
