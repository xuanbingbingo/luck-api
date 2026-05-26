import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

export interface WxpaySettingsValues {
  WxMchId: string
  WxAppId: string
  WxApiV3Key: string
  WxCertSerialNo: string
  WxPrivateKeyPem: string
  WxCertPem: string
  WxPublicKeyID: string
  WxPublicKeyPem: string
  WxMinTopUp: number
}

interface Props {
  defaultValues: WxpaySettingsValues
}

export function WxpaySettingsSection(props: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [loading, setLoading] = useState(false)

  const form = useForm<WxpaySettingsValues>({
    defaultValues: props.defaultValues,
  })

  useEffect(() => {
    form.reset(props.defaultValues)
  }, [props.defaultValues, form])

  const notifyUrl =
    typeof window !== 'undefined'
      ? `${window.location.origin}/api/user/wxpay/notify`
      : '/api/user/wxpay/notify'

  const handleSave = async () => {
    setLoading(true)
    try {
      const values = form.getValues()
      // 非敏感字段：总是提交（后端会返回，前端可预填）
      const options: { key: string; value: string }[] = [
        { key: 'WxMchId', value: values.WxMchId?.trim() || '' },
        { key: 'WxAppId', value: values.WxAppId?.trim() || '' },
        { key: 'WxCertSerialNo', value: values.WxCertSerialNo?.trim() || '' },
        { key: 'WxPublicKeyID', value: values.WxPublicKeyID?.trim() || '' },
        { key: 'WxCertPem', value: values.WxCertPem?.trim() || '' },
        { key: 'WxPublicKeyPem', value: values.WxPublicKeyPem?.trim() || '' },
        { key: 'WxMinTopUp', value: String(values.WxMinTopUp || 1) },
      ]
      // 敏感字段（后端脱敏不回显）：仅在填入新值时提交，留空表示保持不变
      if (values.WxApiV3Key?.trim()) {
        options.push({ key: 'WxApiV3Key', value: values.WxApiV3Key.trim() })
      }
      if (values.WxPrivateKeyPem?.trim()) {
        options.push({
          key: 'WxPrivateKeyPem',
          value: values.WxPrivateKeyPem.trim(),
        })
      }

      for (const opt of options) {
        await updateOption.mutateAsync(opt)
      }
      toast.success(t('Updated successfully'))
    } catch {
      toast.error(t('Update failed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <SettingsSection
      title={t('WeChat Pay (V3 Native)')}
      description={t(
        'Configure WeChat Pay V3 Native (PC scan-to-pay) direct merchant integration'
      )}
    >
      <Alert>
        <AlertDescription className='space-y-1 text-xs'>
          <p>
            {t(
              'Obtain merchant ID, AppID, APIv3 key, certificate serial number, private key, and WeChat Pay public key from the WeChat Pay merchant platform (Account Center → API Security).'
            )}
          </p>
          <p>
            {t('Callback URL (configure in WeChat merchant platform):')}{' '}
            <code className='bg-muted rounded px-1 py-0.5'>{notifyUrl}</code>
          </p>
          <p className='text-muted-foreground'>
            {t(
              'WeChat Pay is enabled automatically once all required fields are filled. No separate switch needed.'
            )}
          </p>
        </AlertDescription>
      </Alert>

      <div className='grid grid-cols-2 gap-4'>
        <div className='grid gap-1.5'>
          <Label>{t('Merchant ID (mch_id)')}</Label>
          <Input placeholder='1743890077' {...form.register('WxMchId')} />
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('AppID')}</Label>
          <Input placeholder='wx...' {...form.register('WxAppId')} />
        </div>
      </div>

      <div className='grid grid-cols-2 gap-4'>
        <div className='grid gap-1.5'>
          <Label>{t('Certificate Serial Number')}</Label>
          <Input {...form.register('WxCertSerialNo')} />
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('WeChat Pay Public Key ID')}</Label>
          <Input placeholder='PUB_KEY_ID_...' {...form.register('WxPublicKeyID')} />
        </div>
      </div>

      <div className='grid gap-1.5'>
        <Label>{t('APIv3 Key')}</Label>
        <Input
          type='password'
          autoComplete='new-password'
          placeholder={t('Configured — leave blank to keep unchanged')}
          {...form.register('WxApiV3Key')}
        />
      </div>

      <div className='grid gap-1.5'>
        <Label>{t('Merchant Private Key (apiclient_key.pem)')}</Label>
        <Textarea
          rows={4}
          className='font-mono text-xs'
          placeholder={t('Configured — leave blank to keep unchanged')}
          {...form.register('WxPrivateKeyPem')}
        />
      </div>

      <div className='grid gap-1.5'>
        <Label>{t('WeChat Pay Public Key (pub_key.pem)')}</Label>
        <Textarea
          rows={4}
          className='font-mono text-xs'
          {...form.register('WxPublicKeyPem')}
        />
      </div>

      <div className='grid gap-1.5'>
        <Label>{t('Merchant Certificate (apiclient_cert.pem, optional)')}</Label>
        <Textarea
          rows={3}
          className='font-mono text-xs'
          {...form.register('WxCertPem')}
        />
      </div>

      <div className='grid grid-cols-2 gap-4'>
        <div className='grid gap-1.5'>
          <Label>{t('Minimum top-up (USD)')}</Label>
          <Input
            type='number'
            min={1}
            {...form.register('WxMinTopUp', { valueAsNumber: true })}
          />
        </div>
      </div>

      <Button onClick={handleSave} disabled={loading}>
        {loading ? t('Saving...') : t('Save Changes')}
      </Button>
    </SettingsSection>
  )
}
