import type { CheckinSettings } from "@/types"

export const checkinSettingsUpdatedEvent = "image-mirror-checkin-settings-updated"

export function emitCheckinSettingsUpdated(settings: CheckinSettings) {
  window.dispatchEvent(new CustomEvent<CheckinSettings>(checkinSettingsUpdatedEvent, { detail: settings }))
}
