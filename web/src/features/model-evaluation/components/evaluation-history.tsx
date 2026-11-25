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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import dayjs from 'dayjs'
import { History, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
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

import { getEvaluationHistory } from '../api'
import { EvaluationStatusBadge } from './status-badge'

export function EvaluationHistory(props: {
  page: number
  selected?: number
  onPage: (page: number) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const userId = useAuthStore((state) => state.auth.user?.id)
  const history = useQuery({
    queryKey: ['evaluations', userId, 'history', props.page],
    queryFn: () => getEvaluationHistory(props.page),
    placeholderData: keepPreviousData,
    refetchInterval: (query) =>
      query.state.data?.items.some((run) => run.status === 'running') &&
      !query.state.error
        ? 5000
        : false,
  })
  return (
    <Card>
      <CardHeader>
        <div className='flex items-center justify-between gap-3'>
          <CardTitle>{t('My test history')}</CardTitle>
          <Button
            variant='outline'
            size='sm'
            disabled={history.isFetching}
            onClick={() => void history.refetch()}
          >
            <RefreshCw className='size-4' aria-hidden />
            {t('Refresh')}
          </Button>
        </div>
        <CardDescription>
          {t(
            'Only your own tests are shown. History remains available after an API key is removed.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>
        {history.isPending && (
          <p role='status' className='text-muted-foreground py-8 text-center'>
            {t('Loading test history...')}
          </p>
        )}
        {history.isError && (
          <p role='alert' className='text-destructive'>
            {t('Could not load test history. Refresh to try again.')}
          </p>
        )}
        {history.data?.items.length === 0 && (
          <EmptyState
            icon={History}
            title={t('No tests yet. Start your first model test.')}
          />
        )}
        <div className='space-y-2'>
          {history.data?.items.map((run) => (
            <Link
              key={run.id}
              to='/model-test'
              search={{ tab: 'history', run: run.id, page: props.page }}
              className={`hover:bg-muted/50 focus-visible:ring-ring flex flex-wrap items-center justify-between gap-3 rounded-lg border p-4 outline-none focus-visible:ring-2 ${props.selected === run.id ? 'border-primary bg-primary/5' : ''}`}
            >
              <div className='min-w-0 space-y-1'>
                <p className='text-sm font-semibold break-all'>
                  {run.model}{' '}
                  <span className='text-muted-foreground font-normal'>
                    · {t(run.question_name)}
                  </span>
                </p>
                <p className='text-muted-foreground text-xs break-words'>
                  {dayjs.unix(run.created_at).format('YYYY-MM-DD HH:mm:ss')} ·{' '}
                  {run.token_name} · {run.token_group || t('Default group')} ·{' '}
                  {run.effort || t('Model default')}
                </p>
              </div>
              <div className='flex flex-wrap items-center gap-3'>
                <span className='text-muted-foreground text-xs'>
                  {run.quota === null
                    ? t('See usage logs')
                    : formatQuotaWithCurrency(run.quota)}
                </span>
                <EvaluationStatusBadge status={run.status} />
              </div>
            </Link>
          ))}
        </div>
        {history.data && history.data.total > 0 && (
          <div className='flex flex-wrap items-center justify-between gap-3 border-t pt-4'>
            <p className='text-muted-foreground text-xs'>
              {t('Total tests')}: {formatNumber(history.data.total, locale)} ·{' '}
              {t('Page')} {formatNumber(props.page, locale)}
            </p>
            <div className='flex gap-2'>
              <Button
                variant='outline'
                size='sm'
                disabled={props.page <= 1 || history.isFetching}
                onClick={() => props.onPage(props.page - 1)}
              >
                {t('Previous')}
              </Button>
              <Button
                variant='outline'
                size='sm'
                disabled={
                  props.page * 10 >= history.data.total || history.isFetching
                }
                onClick={() => props.onPage(props.page + 1)}
              >
                {t('Next')}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
