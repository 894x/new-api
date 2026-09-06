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
import i18next from 'i18next'
import { useCallback, useState } from 'react'
import { toast } from 'sonner'

import {
  getWechatPayStatus,
  isApiSuccess,
  requestWechatPayPayment,
} from '../api'
import { PAYMENT_TYPES } from '../constants'
import type { WechatPayOrder } from '../types'

export function useWechatPayment() {
  const [order, setOrder] = useState<WechatPayOrder | null>(null)
  const [processing, setProcessing] = useState(false)

  const createPayment = useCallback(async (topupAmount: number) => {
    try {
      setProcessing(true)
      const response = await requestWechatPayPayment({
        amount: Math.floor(topupAmount),
        payment_method: PAYMENT_TYPES.WECHAT_NATIVE,
      })
      if (!isApiSuccess(response) || !response.data?.code_url) {
        toast.error(response.message || i18next.t('Payment request failed'))
        return false
      }
      setOrder(response.data)
      return true
    } catch {
      toast.error(i18next.t('Payment request failed'))
      return false
    } finally {
      setProcessing(false)
    }
  }, [])

  const checkStatus = useCallback(async (tradeNo: string) => {
    try {
      const response = await getWechatPayStatus(tradeNo)
      return isApiSuccess(response) && response.data?.status === 'success'
    } catch {
      return false
    }
  }, [])

  const clearPayment = useCallback(() => setOrder(null), [])

  return {
    order,
    processing,
    createPayment,
    checkStatus,
    clearPayment,
  }
}
