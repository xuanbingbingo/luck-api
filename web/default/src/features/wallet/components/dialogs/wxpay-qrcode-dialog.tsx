import { useEffect } from 'react'
import { CheckCircle2, Loader2, QrCode, RefreshCcw } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useTranslation } from 'react-i18next'
import { formatLocalCurrencyAmount } from '@/lib/currency'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { formatCnyAmount } from '../../lib'
import type { WxPayPaymentData } from '../../types'

interface WxPayQRCodeDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  paymentData: WxPayPaymentData | null
  pollStatus: 'paid' | 'expired' | null
  /** 用户主动重启一笔订单（金额不变）。在「过期」时显示重试按钮触发。 */
  onRetry?: () => void
  /** 支付成功 → 上层关弹窗 + 刷余额。组件内通过 useEffect 触发。 */
  onPaid?: () => void
  usdExchangeRate?: number
}

export function WxPayQRCodeDialog({
  open,
  onOpenChange,
  paymentData,
  pollStatus,
  onRetry,
  onPaid,
  usdExchangeRate = 1,
}: WxPayQRCodeDialogProps) {
  const { t } = useTranslation()

  // 检测到支付成功 → 通知上层（刷余额 + 关弹窗）
  useEffect(() => {
    if (pollStatus === 'paid' && onPaid) {
      // 留 800ms 让用户看到成功状态再关
      const timer = setTimeout(() => {
        onPaid()
      }, 800)
      return () => clearTimeout(timer)
    }
  }, [pollStatus, onPaid])

  const codeUrl = paymentData?.code_url ?? ''
  const moneyCny = paymentData?.money_cny ?? 0
  const amount = paymentData?.amount ?? 0
  // 美元换算（仅当用户充值单位是 USD 时才显示括号里的美元金额）
  const usdAmount = amount * usdExchangeRate

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'>
        <AlertDialogHeader>
          <AlertDialogTitle className='flex items-center gap-2 text-xl font-semibold'>
            <QrCode className='h-5 w-5' />
            {t('WeChat Pay')}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t('Scan with WeChat to pay')}
          </AlertDialogDescription>
        </AlertDialogHeader>

        <div className='flex flex-col items-center gap-4 py-2'>
          {/* 双币种金额：人民币（实际收款）+ 美元（用户视角） */}
          <div className='flex flex-col items-center gap-1'>
            <div className='text-3xl font-bold'>¥{moneyCny.toFixed(2)}</div>
            <div className='text-muted-foreground text-sm'>
              {t('Topup Amount')}:{' '}
              {formatLocalCurrencyAmount(usdAmount, {
                digitsLarge: 2,
                digitsSmall: 2,
                abbreviate: false,
              })}
              {' ≈ '}
              {formatCnyAmount(moneyCny)}
            </div>
          </div>

          {/* 二维码 / 成功 / 过期三态切换 */}
          <div className='bg-muted/30 flex h-56 w-56 items-center justify-center rounded-lg border'>
            {pollStatus === 'paid' ? (
              <div className='flex flex-col items-center gap-2'>
                <CheckCircle2 className='h-16 w-16 text-green-600' />
                <span className='font-medium text-green-600'>
                  {t('Payment received')}
                </span>
              </div>
            ) : pollStatus === 'expired' ? (
              <div className='flex flex-col items-center gap-2 px-4 text-center'>
                <RefreshCcw className='text-muted-foreground h-12 w-12' />
                <span className='text-muted-foreground text-sm'>
                  {t('QR code expired, please retry')}
                </span>
              </div>
            ) : codeUrl ? (
              <QRCodeSVG value={codeUrl} size={208} level='M' />
            ) : (
              <Loader2 className='text-muted-foreground h-8 w-8 animate-spin' />
            )}
          </div>

          {/* 状态文案 + 轮询小转圈 */}
          {pollStatus === null && codeUrl && (
            <div className='text-muted-foreground flex items-center gap-2 text-sm'>
              <Loader2 className='h-3 w-3 animate-spin' />
              {t('Waiting for payment...')}
            </div>
          )}
        </div>

        <AlertDialogFooter className='grid grid-cols-2 gap-2 sm:flex'>
          <AlertDialogCancel>
            {pollStatus === 'paid' ? t('Close') : t('Cancel')}
          </AlertDialogCancel>
          {pollStatus === 'expired' && onRetry && (
            <Button onClick={onRetry}>
              <RefreshCcw className='mr-2 h-4 w-4' />
              {t('Retry')}
            </Button>
          )}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
