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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { ArtworkPreview } from '../components/artwork-preview'

test('loads artwork on request and keeps the same sandbox when expanded', async () => {
  const user = userEvent.setup()
  const artwork = '<svg><animate attributeName="opacity" dur="1s" /></svg>'
  render(<ArtworkPreview html={artwork} />)

  expect(screen.queryByTitle('Generated artwork')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Load preview' }))
  const inline = screen.getByTitle('Generated artwork')
  expect(inline).toHaveAttribute('sandbox', 'allow-scripts')
  expect(inline.getAttribute('srcdoc')).toContain(artwork)
  expect(inline.getAttribute('srcdoc')).toContain("connect-src 'none'")

  await user.click(screen.getByRole('button', { name: 'Expand artwork' }))
  const dialog = await screen.findByRole('dialog', {
    name: 'Generated artwork',
  })
  const expanded = within(dialog).getByTitle('Generated artwork')
  expect(expanded).not.toBe(inline)
  expect(expanded).toHaveAttribute('sandbox', 'allow-scripts')
  expect(expanded.getAttribute('srcdoc')).toBe(inline.getAttribute('srcdoc'))

  await user.keyboard('{Escape}')
  await waitFor(() => expect(dialog).not.toBeInTheDocument())
  expect(screen.getByTitle('Generated artwork')).toBeInTheDocument()
})
