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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { getEvaluationReport, rateEvaluation } from '../api'
import type { EvaluationRun } from '../types'
import { ArtworkPreview } from './artwork-preview'
import { EvaluationStatusBadge } from './status-badge'

export function EvaluationReport(props: {
  id: number
  onRepeat: (run: EvaluationRun) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const userId = useAuthStore((state) => state.auth.user?.id)
  const client = useQueryClient()
  const report = useQuery({
    queryKey: ['evaluations', userId, 'report', props.id],
    queryFn: () => getEvaluationReport(props.id),
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.status === 'running' && !query.state.error
        ? 3000
        : false,
  })
  const rating = useMutation({
    mutationFn: (status: string) => rateEvaluation(props.id, status),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ['evaluations', userId] }),
  })
  const run = report.data

  if (report.isPending) {
    return (
      <p role='status' className='text-muted-foreground p-6'>
        {t('Loading test report...')}
      </p>
    )
  }
  if (report.isError || !run) {
    return (
      <Card>
        <CardContent className='space-y-3 pt-6'>
          <p role='alert'>
            {t(
              'Could not load this report. It may not belong to your account.'
            )}
          </p>
          <Button variant='outline' onClick={() => void report.refetch()}>
            {t('Retry')}
          </Button>
        </CardContent>
      </Card>
    )
  }

  const metrics = [
    { label: t('Model'), value: run.model },
    { label: t('API Key'), value: `${run.token_name} · ${run.token_masked}` },
    { label: t('Group'), value: run.token_group || t('Default group') },
    { label: t('Reasoning effort'), value: run.effort || t('Model default') },
    {
      label: t('Input tokens'),
      value: formatNumber(run.prompt_tokens, locale),
    },
    {
      label: t('Output tokens'),
      value: formatNumber(run.completion_tokens, locale),
    },
    {
      label: t('Actual cost'),
      value:
        run.quota === null
          ? t('See usage logs')
          : formatQuotaWithCurrency(run.quota),
    },
    {
      label: t('Duration'),
      value: run.completed_at
        ? `${(run.duration_ms / 1000).toFixed(1)} s`
        : t('In progress'),
    },
  ]

  return (
    <Card>
      <CardHeader>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='space-y-1'>
            <CardTitle>
              {t('Test report')} #{run.id} · {t(run.question_name)}
            </CardTitle>
            <CardDescription>
              {dayjs.unix(run.created_at).format('YYYY-MM-DD HH:mm:ss')} ·{' '}
              {run.question_version} · {run.protocol}
            </CardDescription>
          </div>
          <EvaluationStatusBadge status={run.status} />
        </div>
      </CardHeader>
      <CardContent className='space-y-5'>
        <dl className='grid grid-cols-2 gap-4 rounded-xl border p-4 lg:grid-cols-4'>
          {metrics.map((metric) => (
            <div key={metric.label} className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{metric.label}</dt>
              <dd className='mt-1 text-sm break-words'>{metric.value}</dd>
            </div>
          ))}
        </dl>
        {run.status === 'running' && (
          <p role='status' className='text-muted-foreground text-sm'>
            {t(
              'The test continues in the background. You can leave this page and return through history.'
            )}
          </p>
        )}
        {run.status === 'truncated' && (
          <p
            role='alert'
            className='text-muted-foreground rounded-lg border p-3 text-sm'
          >
            {t(
              'The output reached a limit. This is recorded separately from an incorrect answer.'
            )}
          </p>
        )}
        {run.error && (
          <p
            role='alert'
            className='text-destructive rounded-lg border p-3 text-sm'
          >
            {t(run.error)}
          </p>
        )}
        {run.artwork && (
          <section className='space-y-3'>
            <ArtworkPreview html={run.artwork} />
            <p className='text-muted-foreground text-sm'>
              {t(
                'Rate the composition, requested details and animation. Your review is separate from automatic text grading.'
              )}
            </p>
            <div className='flex flex-wrap gap-2'>
              {(['passed', 'partial', 'failed'] as const).map((status) => (
                <Button
                  key={status}
                  variant={run.status === status ? 'default' : 'outline'}
                  disabled={rating.isPending}
                  onClick={() => rating.mutate(status)}
                >
                  {t(
                    {
                      passed: 'Meets requirements',
                      partial: 'Partly meets requirements',
                      failed: 'Does not meet requirements',
                    }[status]
                  )}
                </Button>
              ))}
            </div>
          </section>
        )}
        <details className='rounded-lg border p-4' open>
          <summary className='cursor-pointer font-medium'>
            {t('Full test question')}
          </summary>
          <pre className='mt-3 max-h-80 overflow-auto text-sm break-words whitespace-pre-wrap'>
            {run.prompt}
          </pre>
        </details>
        {run.expected && run.status !== 'running' && (
          <div className='rounded-lg border p-4'>
            <h3 className='font-medium'>{t('Expected answer')}</h3>
            <pre className='mt-2 text-sm break-words whitespace-pre-wrap'>
              {run.expected}
            </pre>
            <p className='text-muted-foreground mt-2 text-xs'>
              {t(
                'The full JSON structure, values and array order must match. Extra text does not pass.'
              )}
            </p>
          </div>
        )}
        {run.answer && (
          <details className='rounded-lg border p-4' open={run.kind === 'text'}>
            <summary className='cursor-pointer font-medium'>
              {t('Original model response')}
            </summary>
            <pre className='mt-3 max-h-[32rem] overflow-auto text-sm break-words whitespace-pre-wrap'>
              {run.answer}
            </pre>
          </details>
        )}
        {run.request_id && (
          <p className='text-muted-foreground text-xs break-all'>
            {t('Request ID')}: {run.request_id} · {t('Finish reason')}:{' '}
            {run.finish_reason || '—'}
          </p>
        )}
        <p className='text-muted-foreground text-xs'>
          {t(
            'Actual cost is matched to consumption logs by request ID. Missing cost does not mean the test was free.'
          )}
        </p>
        <div className='flex flex-wrap gap-2'>
          <Button
            variant='outline'
            disabled={run.status === 'running'}
            onClick={() => props.onRepeat(run)}
          >
            <RotateCcw className='size-4' aria-hidden />
            {t('Repeat with these settings')}
          </Button>
          <Button
            variant='ghost'
            disabled={report.isFetching}
            onClick={() => void report.refetch()}
          >
            {t('Refresh report')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
