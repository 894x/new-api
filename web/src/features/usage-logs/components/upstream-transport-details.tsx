import { useTranslation } from 'react-i18next'
import type { UpstreamTransportInfo } from '../types'

export function UpstreamTransportDetails({
  attempts,
}: {
  attempts: UpstreamTransportInfo[]
}) {
  const { t } = useTranslation()
  if (!attempts.length) return null
  return (
    <section className='space-y-2'>
      <h3 className='font-medium'>{t('Upstream transport attempts')}</h3>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Write time starts after connection acquisition and includes transport scheduling and flow control; it is not pure network upload time. At most 8 attempts are retained.'
        )}
      </p>
      {attempts.map((attempt) => (
        <div key={attempt.attempt} className='rounded-md border p-3 text-xs'>
          <div className='mb-2 font-medium'>
            #{attempt.attempt} · {t('Channel ID')} {attempt.channel_id} ·{' '}
            {attempt.protocol || '—'} · {attempt.outcome}
          </div>
          <dl className='grid grid-cols-2 gap-2 sm:grid-cols-3'>
            {[
              [
                t('Request body bytes'),
                attempt.body_bytes < 0
                  ? t('Unknown')
                  : attempt.body_bytes.toLocaleString(),
              ],
              [
                t('Threshold bytes'),
                attempt.threshold_bytes?.toLocaleString() ?? '—',
              ],
              [t('Routing reason'), `${attempt.reason} → ${attempt.policy}`],
              [t('Connection reused'), attempt.reused ? t('Yes') : t('No')],
              [
                t('Connection acquisition (ms)'),
                attempt.acquire_ms?.toFixed(2) ?? '—',
              ],
              [
                t('Write after acquisition (ms)'),
                attempt.write_ms?.toFixed(2) ?? '—',
              ],
              [
                'DNS / TCP / TLS (ms)',
                [attempt.dns_ms, attempt.tcp_ms, attempt.tls_ms]
                  .map((v) => v?.toFixed(2) ?? '—')
                  .join(' / '),
              ],
              [
                t('Wait for first byte (ms)'),
                attempt.first_byte_ms?.toFixed(2) ?? '—',
              ],
              [
                t('Acquisitions / writes'),
                `${attempt.connection_acquisitions} / ${attempt.write_callbacks}`,
              ],
            ].map(([label, value]) => (
              <div key={label}>
                <dt className='text-muted-foreground'>{label}</dt>
                <dd className='mt-1 break-all tabular-nums'>{value}</dd>
              </div>
            ))}
          </dl>
        </div>
      ))}
    </section>
  )
}
