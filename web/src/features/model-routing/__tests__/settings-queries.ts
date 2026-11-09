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
import { fireEvent, screen, within } from '@testing-library/react'

// Follow the same gear-button interaction for fields rendered in a portal.
export function getSettingsField(
  role: 'spinbutton' | 'switch' | 'combobox' | 'button',
  options: { name: string; hidden?: boolean },
  container?: HTMLElement
): HTMLElement {
  if (!options.hidden) {
    const visible = screen.queryAllByRole(role, options)
    if (!container && visible.length === 1) {
      const popup = visible[0].closest('[data-slot="popover-content"]')
      if (!popup || popup.hasAttribute('data-open')) return visible[0]
    }
    let subject = 'kimi-k3'
    for (const prefix of [
      'Priority for ',
      'Weight for ',
      'Retries for ',
      'Early chunk for ',
    ]) {
      if (options.name.startsWith(prefix)) {
        subject = options.name.slice(prefix.length)
      }
    }
    if (options.name.startsWith('Enable ')) {
      subject = options.name.slice('Enable '.length, -' in this pool'.length)
    }
    const trigger = within(container ?? document.body).getByRole('button', {
      name: `Settings: ${subject}`,
    })
    if (trigger.getAttribute('aria-expanded') !== 'true') {
      fireEvent.click(trigger)
    }
    const panelId = trigger.getAttribute('aria-controls')
    const panel = document.querySelector<HTMLElement>(`[id="${panelId}"]`)
    if (panel) return within(panel).getByRole(role, options)
  }
  return screen.getByRole(role, options)
}
