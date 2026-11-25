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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, expect, test } from 'vitest'

import type { AnnouncementItem } from '@/features/dashboard/types'
import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useNotificationStore } from '@/stores/notification-store'

import { LatestAnnouncementDialog } from '../latest-announcement-dialog'

const previous = {
  id: 1,
  publishDate: '2026-11-24T08:00:00Z',
  content: 'Previous announcement',
}
const latest = {
  id: 2,
  publishDate: '2026-11-25T08:00:00Z',
  content: '**Latest service update**',
  extra: 'Maintenance window details',
}

const getAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterAll(() => {
  if (getAnimations) {
    Object.defineProperty(HTMLElement.prototype, 'getAnimations', getAnimations)
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

function renderWithAnnouncements(
  announcements: AnnouncementItem[],
  enabled = true
) {
  const client = new QueryClient()
  client.setQueryData(STATUS_QUERY_KEY, {
    announcements_enabled: enabled,
    announcements,
  })
  return render(
    <QueryClientProvider client={client}>
      <LatestAnnouncementDialog />
    </QueryClientProvider>
  )
}

afterEach(() => {
  useNotificationStore.setState({ dismissedAnnouncementPopupKeys: [] })
  useNotificationStore.persist.clearStorage()
})

test('shows only the latest announcement and Close allows it to appear on the next visit', async () => {
  const user = userEvent.setup()
  const visit = renderWithAnnouncements([latest, previous])
  const dialog = screen.getByRole('dialog', { name: 'Latest announcement' })
  expect(within(dialog).getByText('Latest service update')).toBeVisible()
  expect(within(dialog).getByText('Maintenance window details')).toBeVisible()
  expect(screen.queryByText(previous.content)).not.toBeInTheDocument()

  await user.click(within(dialog).getByRole('button', { name: 'Close' }))
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

  visit.unmount()
  renderWithAnnouncements([latest, previous])
  expect(
    screen.getByRole('dialog', { name: 'Latest announcement' })
  ).toBeVisible()
})

test('persists dismissal across visits and allows a new announcement even if the ID is reused', async () => {
  const user = userEvent.setup()
  const visit = renderWithAnnouncements([latest, previous])
  await user.click(
    screen.getByRole('button', { name: 'Do not show this announcement again' })
  )
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  visit.unmount()

  const persisted = localStorage.getItem('notification-storage')
  if (!persisted) throw new Error('Dismissal was not persisted')
  useNotificationStore.setState({ dismissedAnnouncementPopupKeys: [] })
  localStorage.setItem('notification-storage', persisted)
  await useNotificationStore.persist.rehydrate()

  const revisit = renderWithAnnouncements([latest, previous])
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  revisit.unmount()

  const newAnnouncement = {
    ...latest,
    content: 'A newly published announcement',
    publishDate: '2026-11-26T08:00:00Z',
  }
  renderWithAnnouncements([newAnnouncement, previous])
  expect(screen.getByText(newAnnouncement.content)).toBeVisible()
})

test.each([
  { announcements: [latest], enabled: false },
  { announcements: [], enabled: true },
])(
  'stays closed when announcements are disabled or empty: $enabled',
  ({ announcements, enabled }) => {
    renderWithAnnouncements(announcements, enabled)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  }
)
