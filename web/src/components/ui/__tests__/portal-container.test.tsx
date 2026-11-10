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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { Dialog } from '@/components/dialog'

import { Combobox } from '../combobox'
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerTitle,
} from '../drawer'
import { Popover, PopoverContent, PopoverTrigger } from '../popover'
import { PortalContainerContext } from '../portal-container'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../select'

const options = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'gemini', label: 'Google' },
]

function ProviderCombobox(props: {
  onValueChange: (value: string | null) => void
  allowCustomValue?: boolean
}) {
  return (
    <Combobox
      options={options}
      value='openai'
      onValueChange={props.onValueChange}
      allowCustomValue={props.allowCustomValue}
      aria-label='Provider'
    />
  )
}

function ProviderSelect(props: {
  onValueChange: (value: string | null) => void
}) {
  return (
    <Select value='openai' onValueChange={props.onValueChange}>
      <SelectTrigger aria-label='Provider'>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function FilterDrawer(props: { children: ReactNode }) {
  return (
    <Drawer open>
      <DrawerContent>
        <DrawerTitle>Filters</DrawerTitle>
        <DrawerDescription>Adjust the result filters</DrawerDescription>
        {props.children}
      </DrawerContent>
    </Drawer>
  )
}

// JSDOM does not implement the pointer-capture API used by the drawer.
const pointerCapture = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'setPointerCapture'
)

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'setPointerCapture', {
    configurable: true,
    value: () => undefined,
  })
})

afterEach(() => {
  if (pointerCapture) {
    Object.defineProperty(
      HTMLElement.prototype,
      'setPointerCapture',
      pointerCapture
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'setPointerCapture')
  }
})

describe('popups inside a drawer', () => {
  it('anchors editable combobox options in the transformed drawer coordinate space', async () => {
    const change = vi.fn()
    render(
      <FilterDrawer>
        <ProviderCombobox allowCustomValue onValueChange={change} />
      </FilterDrawer>
    )
    const user = userEvent.setup()
    const dialog = screen.getByRole('dialog', { name: 'Filters' })
    const input = within(dialog).getByRole('combobox', { name: 'Provider' })
    dialog.style.transform = 'translateY(0)'
    dialog.scrollTop = 24
    dialog.scrollLeft = 3
    vi.spyOn(dialog, 'getBoundingClientRect').mockReturnValue(
      new DOMRect(10, 300, 360, 500)
    )
    vi.spyOn(input, 'getBoundingClientRect').mockReturnValue(
      new DOMRect(20, 420, 320, 32)
    )

    await user.click(input)
    const listbox = await within(dialog).findByRole('listbox')

    expect(listbox.parentElement).toHaveStyle({
      position: 'absolute',
      top: '180px',
      left: '13px',
      width: '320px',
    })
    await user.click(within(listbox).getByRole('option', { name: 'Google' }))
    expect(change).toHaveBeenCalledWith('gemini')
    expect(dialog).toBeInTheDocument()
  })

  it('renders combobox options inside the drawer dialog and clicking one applies it without closing the drawer', async () => {
    const change = vi.fn()
    render(
      <FilterDrawer>
        <ProviderCombobox onValueChange={change} />
      </FilterDrawer>
    )
    const user = userEvent.setup()
    const dialog = screen.getByRole('dialog', { name: 'Filters' })

    await user.click(within(dialog).getByRole('combobox', { name: 'Provider' }))
    await user.click(
      await within(dialog).findByRole('option', { name: 'Google' })
    )

    expect(change).toHaveBeenCalledWith('gemini')
    expect(screen.getByRole('dialog', { name: 'Filters' })).toBeInTheDocument()
  })

  it('renders select options inside the drawer dialog and clicking one applies it without closing the drawer', async () => {
    const change = vi.fn()
    render(
      <FilterDrawer>
        <ProviderSelect onValueChange={change} />
      </FilterDrawer>
    )
    const user = userEvent.setup()
    const dialog = screen.getByRole('dialog', { name: 'Filters' })

    await user.click(within(dialog).getByRole('combobox', { name: 'Provider' }))
    await user.click(
      await within(dialog).findByRole('option', { name: 'Google' })
    )

    expect(change).toHaveBeenCalledWith('gemini', expect.anything())
    expect(screen.getByRole('dialog', { name: 'Filters' })).toBeInTheDocument()
  })
})

describe('popups outside a drawer', () => {
  it('keeps a mounted popover portal out of layout before opening and after closing', async () => {
    render(
      <Popover>
        <PopoverTrigger>Channel settings</PopoverTrigger>
        <PopoverContent keepMounted>
          <input aria-label='Channel weight' defaultValue='10' />
        </PopoverContent>
      </Popover>
    )
    const portal = document.body.querySelector('[data-base-ui-portal]')
    // The floating positioner is out of flow; its portal wrapper must not
    // become an extra grid item, even while the popup stays mounted.
    expect(portal).toHaveClass('contents')
    const user = userEvent.setup()
    const trigger = screen.getByRole('button', { name: 'Channel settings' })
    await user.click(trigger)
    const weight = screen.getByRole('textbox', { name: 'Channel weight' })
    await user.clear(weight)
    await user.type(weight, '25')
    expect(weight).toHaveValue('25')
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    await user.keyboard('{Escape}')
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(document.body).toContainElement(portal as HTMLElement)
    expect(portal).toHaveClass('contents')
  })

  it('keeps a dialog selector interactive when its parent supplies a background popup container', async () => {
    const container = createRef<HTMLDivElement>()
    const change = vi.fn()
    render(
      <div ref={container}>
        <PortalContainerContext.Provider value={container}>
          <Dialog defaultOpen title='Edit strategy'>
            <ProviderSelect onValueChange={change} />
          </Dialog>
        </PortalContainerContext.Provider>
      </div>
    )
    const user = userEvent.setup()
    const dialog = screen.getByRole('dialog', { name: 'Edit strategy' })
    await user.click(within(dialog).getByRole('combobox', { name: 'Provider' }))
    await user.click(await screen.findByRole('option', { name: 'Google' }))
    expect(change).toHaveBeenCalledWith('gemini', expect.anything())
    expect(dialog).toBeInTheDocument()
  })

  it('anchors editable combobox options in viewport coordinates outside dialogs', async () => {
    render(<ProviderCombobox allowCustomValue onValueChange={vi.fn()} />)
    const user = userEvent.setup()
    const input = screen.getByRole('combobox', { name: 'Provider' })
    vi.spyOn(input, 'getBoundingClientRect').mockReturnValue(
      new DOMRect(20, 420, 320, 32)
    )

    await user.click(input)
    const listbox = await screen.findByRole('listbox')

    expect(listbox.parentElement).toHaveStyle({
      position: 'fixed',
      top: '456px',
      left: '20px',
      width: '320px',
    })
  })

  it('portals combobox options to document.body outside the component subtree', async () => {
    const view = render(<ProviderCombobox onValueChange={vi.fn()} />)
    const user = userEvent.setup()

    await user.click(screen.getByRole('combobox', { name: 'Provider' }))
    const listbox = await screen.findByRole('listbox')

    expect(document.body).toContainElement(listbox)
    expect(view.container).not.toContainElement(listbox)
  })

  it('portals select options to document.body outside the component subtree', async () => {
    const view = render(<ProviderSelect onValueChange={vi.fn()} />)
    const user = userEvent.setup()

    await user.click(screen.getByRole('combobox', { name: 'Provider' }))
    const listbox = await screen.findByRole('listbox')

    expect(document.body).toContainElement(listbox)
    expect(view.container).not.toContainElement(listbox)
  })
})
