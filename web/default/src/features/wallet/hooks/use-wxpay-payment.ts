import { useState, useCallback, useEffect, useRef } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import { isApiSuccess, queryWxPayOrder, requestWxPayPayment } from '../api'
import type { WxPayPaymentData } from '../types'

// 前端轮询间隔。订单一旦支付，notify + 主动查询双重保险下，2 秒拿到结果体感顺畅。
const POLL_INTERVAL_MS = 2000

// 二维码有效期 5 分钟，超过自动停止轮询并提示用户重新发起。
const POLL_MAX_DURATION_MS = 5 * 60 * 1000

interface UseWxPayPaymentResult {
  /** 当前正在创建订单（点击 → 拿 code_url 期间） */
  creating: boolean
  /** 当前订单数据（包含 code_url 和 trade_no），未开始为 null */
  paymentData: WxPayPaymentData | null
  /** 轮询拿到的最终状态：null=尚未支付，'paid'=已支付，'expired'=超时 */
  pollStatus: 'paid' | 'expired' | null
  /** 用户点击微信支付按钮的入口 */
  startWxPayment: (amount: number) => Promise<boolean>
  /** 主动取消（关闭弹窗时调用） */
  cancel: () => void
}

/**
 * 微信支付 V3 Native：下单 + 弹窗 + 轮询全套流程的状态机。
 *
 * 流程：
 *   1. startWxPayment(amount) → 调 /wxpay/pay → 拿 code_url
 *   2. paymentData 不为 null → 上层组件用它渲染二维码弹窗
 *   3. 每 2s 调 /wxpay/query/:trade_no
 *   4. 拿到 paid=true → pollStatus='paid' → 上层关弹窗 + 刷余额
 *   5. 超 5 分钟 → pollStatus='expired'
 */
export function useWxPayPayment(): UseWxPayPaymentResult {
  const [creating, setCreating] = useState(false)
  const [paymentData, setPaymentData] = useState<WxPayPaymentData | null>(null)
  const [pollStatus, setPollStatus] = useState<'paid' | 'expired' | null>(null)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const startedAtRef = useRef<number>(0)

  const stopPolling = useCallback(() => {
    if (timerRef.current) {
      clearInterval(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const cancel = useCallback(() => {
    stopPolling()
    setPaymentData(null)
    setPollStatus(null)
  }, [stopPolling])

  // 副作用：paymentData 出现时启动轮询
  useEffect(() => {
    if (!paymentData) {
      stopPolling()
      return
    }

    startedAtRef.current = Date.now()
    setPollStatus(null)

    const tick = async () => {
      if (Date.now() - startedAtRef.current > POLL_MAX_DURATION_MS) {
        setPollStatus('expired')
        stopPolling()
        return
      }
      try {
        const resp = await queryWxPayOrder(paymentData.trade_no)
        if (isApiSuccess(resp) && resp.data?.paid) {
          setPollStatus('paid')
          stopPolling()
        }
      } catch {
        // 网络抖动一次，不要停轮询
      }
    }

    // 立即查一次，再开始按节奏轮询
    void tick()
    timerRef.current = setInterval(tick, POLL_INTERVAL_MS)

    return () => {
      stopPolling()
    }
  }, [paymentData, stopPolling])

  const startWxPayment = useCallback(async (amount: number) => {
    setCreating(true)
    try {
      const resp = await requestWxPayPayment({ amount })
      if (!isApiSuccess(resp) || !resp.data?.code_url) {
        toast.error(
          resp.message || i18next.t('Payment request failed')
        )
        return false
      }
      setPaymentData(resp.data)
      return true
    } catch {
      toast.error(i18next.t('Payment request failed'))
      return false
    } finally {
      setCreating(false)
    }
  }, [])

  return { creating, paymentData, pollStatus, startWxPayment, cancel }
}
