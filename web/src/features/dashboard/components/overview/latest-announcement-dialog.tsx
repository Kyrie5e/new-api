/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'

import type { AnnouncementItem } from '@/features/dashboard/types'
import { statusQueryOptions, type StatusData } from '@/lib/status-query'
import { useNotificationStore } from '@/stores/notification-store'

import { AnnouncementDetailModal } from './announcement-detail-dialog'

function selectLatestAnnouncement(status: StatusData | null) {
  if (!status?.announcements_enabled || !Array.isArray(status.announcements)) {
    return null
  }

  // The status API sorts announcements by publication date, newest first.
  const [latest] = status.announcements as AnnouncementItem[]
  if (!latest?.content?.trim()) return null

  // IDs can be reused after deletion. Legacy entries may have no ID at all.
  const key = JSON.stringify([latest.publishDate, latest.id ?? latest.content])
  return { announcement: latest, key }
}

export function LatestAnnouncementDialog() {
  const { data: latest } = useQuery({
    ...statusQueryOptions,
    select: selectLatestAnnouncement,
  })
  const [closedForVisit, setClosedForVisit] = useState<string | null>(null)
  const dismissed = useNotificationStore((state) =>
    latest ? state.dismissedAnnouncementPopupKeys.includes(latest.key) : false
  )
  const dismiss = useNotificationStore(
    (state) => state.dismissAnnouncementPopup
  )

  if (!latest) return null

  return (
    <AnnouncementDetailModal
      open={!dismissed && closedForVisit !== latest.key}
      onOpenChange={(open) => {
        if (!open) setClosedForVisit(latest.key)
      }}
      onDismissReminder={() => dismiss(latest.key)}
      announcement={latest.announcement}
    />
  )
}
