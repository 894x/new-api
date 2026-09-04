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
import { Loader2 } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

import type { WechatPayOrder } from '../../types'

interface WechatPayDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  order: WechatPayOrder | null
  checkStatus: (tradeNo: string) => Promise<boolean>
  onPaid: () => void
}

export function WechatPayDialog({
  open,
  onOpenChange,
  order,
  checkStatus,
  onPaid,
}: WechatPayDialogProps) {
  const { t } = useTranslation()
  const [checking, setChecking] = useState(false)
  const [expired, setExpired] = useState(false)

  useEffect(() => {
    if (!open || !order) return

    let stopped = false
    let inFlight = false
    let completed = false
    const check = async () => {
      if (stopped || inFlight || completed) return
      if (Date.now() >= order.expires_at * 1000) {
        setExpired(true)
        return
      }
      inFlight = true
      setChecking(true)
      try {
        if ((await checkStatus(order.trade_no)) && !stopped) {
          completed = true
          onPaid()
        }
      } finally {
        inFlight = false
        if (!stopped) setChecking(false)
      }
    }

    setExpired(false)
    void check()
    const interval = window.setInterval(() => void check(), 3000)
    return () => {
      stopped = true
      window.clearInterval(interval)
    }
  }, [checkStatus, onPaid, open, order])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Scan with WeChat Pay')}</DialogTitle>
          <DialogDescription>
            {t('Use WeChat to scan the QR code and complete payment.')}
          </DialogDescription>
        </DialogHeader>

        {order ? (
          <div className='flex flex-col items-center gap-4 py-2'>
            <div className='rounded-xl border bg-white p-4 shadow-sm'>
              <QRCodeSVG
                value={order.code_url}
                size={220}
                level='M'
                title={t('WeChat Pay QR code')}
              />
            </div>
            <div className='text-center'>
              <div className='text-2xl font-semibold'>
                ¥{(order.money_cents / 100).toFixed(2)}
              </div>
              <div className='text-muted-foreground mt-1 font-mono text-xs'>
                {order.trade_no}
              </div>
            </div>
            <Alert>
              <AlertDescription className='flex items-center gap-2'>
                {checking && !expired ? (
                  <Loader2 className='h-4 w-4 animate-spin' />
                ) : null}
                {expired
                  ? t(
                      'This payment QR code has expired. Please create a new order.'
                    )
                  : t('Waiting for payment confirmation...')}
              </AlertDescription>
            </Alert>
          </div>
        ) : null}

        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
