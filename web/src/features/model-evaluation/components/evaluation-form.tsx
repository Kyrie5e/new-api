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
import { zodResolver } from '@hookform/resolvers/zod'
import { useInfiniteQuery, useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArrowRight, Loader2, RefreshCw } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getApiKeys } from '@/features/keys/api'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  getEvaluationModels,
  getEvaluationQuestions,
  startEvaluation,
} from '../api'
import type { EvaluationRun } from '../types'

const evaluationSchema = z.object({
  token_id: z.number().int().positive(),
  model: z.string().min(1),
  question_id: z.string().min(1),
  protocol: z.enum(['chat', 'responses']),
  effort: z.enum(['', 'low', 'medium', 'high']),
  confirm_charge: z.boolean().refine(Boolean),
})

type EvaluationValues = z.infer<typeof evaluationSchema>

export function EvaluationForm(props: {
  source?: EvaluationRun
  onStarted: (id: number) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const userId = useAuthStore((state) => state.auth.user?.id)
  const form = useForm<EvaluationValues>({
    resolver: zodResolver(evaluationSchema),
    defaultValues: {
      token_id: props.source?.token_id ?? 0,
      model: props.source?.model ?? '',
      question_id: props.source?.question_id ?? 'logic',
      protocol: props.source?.protocol ?? 'chat',
      effort: (props.source?.effort ?? '') as EvaluationValues['effort'],
      confirm_charge: false,
    },
  })
  const values = form.watch()
  const keys = useInfiniteQuery({
    queryKey: ['evaluations', userId, 'keys'],
    initialPageParam: 1,
    queryFn: async ({ pageParam }) => {
      const response = await getApiKeys({ p: pageParam, size: 100 })
      if (!response.success || !response.data) {
        throw new Error(response.message || 'Cannot load API keys')
      }
      return response.data
    },
    getNextPageParam: (last, pages) =>
      pages.flatMap((page) => page.items).length < last.total
        ? pages.length + 1
        : undefined,
    retry: false,
  })
  const tokens = keys.data?.pages.flatMap((page) => page.items) ?? []
  const token = tokens.find((item) => item.id === values.token_id)
  const tokenAvailable = (item: (typeof tokens)[number]): boolean =>
    item.status === 1 &&
    (item.expired_time === -1 || item.expired_time > Date.now() / 1000) &&
    (item.unlimited_quota || item.remain_quota > 0)
  const keyOptions = tokens.map((item) => ({
    value: String(item.id),
    label: `${item.name || t('Unnamed API key')} · ${item.key} · ${item.group || t('Default group')}${tokenAvailable(item) ? '' : ` · ${t('Unavailable')}`}`,
    disabled: !tokenAvailable(item),
  }))
  const questions = useQuery({
    queryKey: ['evaluation-questions'],
    queryFn: getEvaluationQuestions,
  })
  const models = useQuery({
    queryKey: ['evaluations', userId, 'models', values.token_id],
    queryFn: () => getEvaluationModels(values.token_id),
    enabled: Boolean(token && tokenAvailable(token)),
    retry: false,
  })
  const start = useMutation({
    mutationFn: startEvaluation,
    retry: false,
    onSuccess: (run) => props.onStarted(run.id),
  })
  const question = questions.data?.find(
    (item) => item.id === values.question_id
  )
  const questionGroups = [
    {
      kind: 'text' as const,
      title: t('Text tests'),
      description: t('Responses are checked against a versioned answer key.'),
    },
    {
      kind: 'artwork' as const,
      title: t('Visual creation'),
      description: t('Inspect the isolated result and rate it yourself.'),
    },
  ]
  const modelOptions = (models.data ?? []).map((model) => ({
    value: model,
    label: model,
  }))

  useEffect(() => {
    form.setValue('confirm_charge', false)
  }, [
    values.token_id,
    values.model,
    values.question_id,
    values.protocol,
    values.effort,
    form,
  ])

  function submit(input: EvaluationValues): void {
    const sourceId =
      props.source?.question_id === input.question_id
        ? props.source.id
        : undefined
    start.mutate({ ...input, source_id: sourceId })
  }

  return (
    <form
      className='grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_20rem]'
      onSubmit={form.handleSubmit(submit)}
    >
      <Card>
        <CardHeader>
          <CardTitle>{t('Choose a test')}</CardTitle>
          <CardDescription>
            {t(
              'Text tests are graded automatically. Animation tests are reviewed by you.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-4'>
          {questions.isPending && (
            <p role='status'>{t('Loading test questions...')}</p>
          )}
          {questions.isError && (
            <Button
              type='button'
              variant='outline'
              onClick={() => void questions.refetch()}
            >
              {t('Retry')}
            </Button>
          )}
          {questionGroups.map((group) => {
            const items =
              questions.data?.filter((item) => item.kind === group.kind) ?? []
            if (items.length === 0) return null
            return (
              <section
                key={group.kind}
                className='space-y-3'
                aria-label={group.title}
              >
                <div>
                  <h3 className='text-sm font-semibold'>{group.title}</h3>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {group.description}
                  </p>
                </div>
                <div
                  className='grid gap-3 sm:grid-cols-2'
                  role='group'
                  aria-label={group.title}
                >
                  {items.map((item) => (
                    <button
                      key={item.id}
                      type='button'
                      aria-pressed={values.question_id === item.id}
                      className={cn(
                        'border-input hover:border-primary/50 focus-visible:ring-ring rounded-lg border p-4 text-left transition-colors outline-none focus-visible:ring-2',
                        values.question_id === item.id &&
                          'border-primary bg-primary/5'
                      )}
                      onClick={() => form.setValue('question_id', item.id)}
                    >
                      <span className='text-muted-foreground text-xs'>
                        {item.kind === 'text'
                          ? t('Automatic grading')
                          : t('Visual review')}
                      </span>
                      <span className='mt-1 block font-semibold'>
                        {t(item.name)}
                      </span>
                      <span className='text-muted-foreground mt-2 block text-sm'>
                        {t(item.description)}
                      </span>
                      <span className='text-muted-foreground mt-3 block text-xs'>
                        {item.version}
                      </span>
                    </button>
                  ))}
                </div>
              </section>
            )
          })}
          {question && (
            <section
              className='bg-muted/30 space-y-3 rounded-lg border p-4'
              aria-live='polite'
              aria-label={t('Selected test details')}
            >
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <div className='min-w-0'>
                  <h3 className='font-semibold'>{t(question.name)}</h3>
                  <p className='text-muted-foreground mt-1 text-sm'>
                    {t(question.description)}
                  </p>
                </div>
                <Badge variant='outline'>
                  {question.kind === 'text'
                    ? t('Automatic grading')
                    : t('Visual review')}
                </Badge>
              </div>
              <div className='flex flex-wrap gap-x-6 gap-y-2 border-t pt-3 text-sm'>
                <p>
                  <span className='text-muted-foreground'>
                    {t('Maximum output')}:
                  </span>
                  {formatNumber(question.max_output, locale)} tokens
                </p>
                <p>
                  <span className='text-muted-foreground'>
                    {t('Question version')}:
                  </span>
                  {question.version}
                </p>
              </div>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Actual usage and cost vary by model response. Billing uses the selected API key at the normal site rate.'
                )}
              </p>
            </section>
          )}
          <div className='bg-muted/50 text-muted-foreground rounded-lg p-4 text-sm'>
            {t(
              'A test reflects one response, not proof of model identity. Compare repeated runs with the same settings.'
            )}
          </div>
          {props.source && values.question_id === props.source.question_id && (
            <p className='text-primary text-sm'>
              {t('Repeating the exact question from report')} #{props.source.id}
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Test configuration')}</CardTitle>
          <CardDescription>
            {t('Use an API key you already created on this site.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-4'>
          <div className='space-y-2'>
            <div className='flex items-center justify-between'>
              <Label htmlFor='evaluation-key'>{t('API Key')}</Label>
              <Button
                type='button'
                variant='ghost'
                size='icon'
                aria-label={t('Refresh API keys')}
                disabled={keys.isFetching}
                onClick={() => void keys.refetch()}
              >
                <RefreshCw className='size-4' aria-hidden />
              </Button>
            </div>
            <Select
              items={keyOptions}
              value={values.token_id ? String(values.token_id) : null}
              onValueChange={(value) => {
                form.setValue('token_id', Number(value))
                form.setValue('model', '')
              }}
            >
              <SelectTrigger
                id='evaluation-key'
                className='h-10 w-full'
                aria-invalid={Boolean(form.formState.errors.token_id)}
              >
                <SelectValue placeholder={t('Select an API key')} />
              </SelectTrigger>
              <SelectContent>
                {keyOptions.map((item) => (
                  <SelectItem
                    key={item.value}
                    value={item.value}
                    disabled={item.disabled}
                  >
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {keys.isPending && (
              <p role='status' className='text-muted-foreground text-xs'>
                {t('Loading API keys...')}
              </p>
            )}
            {keys.isError && (
              <p role='alert' className='text-destructive text-xs'>
                {t('Could not load API keys. Refresh to try again.')}
              </p>
            )}
            {keys.hasNextPage && (
              <Button
                type='button'
                variant='link'
                disabled={keys.isFetchingNextPage}
                onClick={() => void keys.fetchNextPage()}
              >
                {t('Load more API keys')}
              </Button>
            )}
            {!keys.isPending && !keys.isError && tokens.length === 0 && (
              <Link to='/keys' className='text-primary text-sm underline'>
                {t('Create an API key first')}
              </Link>
            )}
            {token && (
              <p className='text-muted-foreground text-xs'>
                {t('Available quota')}:{' '}
                {token.unlimited_quota
                  ? t('Unlimited')
                  : formatQuotaWithCurrency(token.remain_quota)}
              </p>
            )}
          </div>
          <div className='space-y-2'>
            <Label htmlFor='evaluation-model'>{t('Model')}</Label>
            <Select
              items={modelOptions}
              value={values.model || null}
              onValueChange={(value) => form.setValue('model', value ?? '')}
              disabled={!token || !tokenAvailable(token) || models.isFetching}
            >
              <SelectTrigger
                id='evaluation-model'
                className='h-10 w-full'
                aria-invalid={Boolean(form.formState.errors.model)}
              >
                <SelectValue placeholder={t('Select a model')} />
              </SelectTrigger>
              <SelectContent>
                {modelOptions.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {models.isFetching && (
              <p role='status' className='text-muted-foreground text-xs'>
                {t('Loading available models...')}
              </p>
            )}
            {models.isError && (
              <p role='alert' className='text-destructive text-xs'>
                {t('Check the API key status, group and IP restrictions.')}{' '}
                <Button
                  type='button'
                  variant='link'
                  onClick={() => void models.refetch()}
                >
                  {t('Retry')}
                </Button>
              </p>
            )}
            {models.isSuccess && models.data.length === 0 && (
              <p className='text-muted-foreground text-xs'>
                {t('No models available for this API key.')}
              </p>
            )}
          </div>
          <details className='rounded-lg border p-3'>
            <summary className='cursor-pointer text-sm font-medium'>
              {t('Advanced settings')}
            </summary>
            <div className='mt-3 space-y-3'>
              <div className='space-y-2'>
                <Label htmlFor='evaluation-protocol'>{t('API protocol')}</Label>
                <Select
                  items={[
                    { value: 'chat', label: 'Chat Completions' },
                    { value: 'responses', label: 'Responses' },
                  ]}
                  value={values.protocol}
                  onValueChange={(value) =>
                    form.setValue(
                      'protocol',
                      value === 'responses' ? 'responses' : 'chat'
                    )
                  }
                >
                  <SelectTrigger id='evaluation-protocol' className='w-full'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='chat'>Chat Completions</SelectItem>
                    <SelectItem value='responses'>Responses</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className='space-y-2'>
                <Label htmlFor='evaluation-effort'>
                  {t('Reasoning effort')}
                </Label>
                <Select
                  items={[
                    { value: '', label: t('Model default') },
                    { value: 'low', label: t('Low') },
                    { value: 'medium', label: t('Medium') },
                    { value: 'high', label: t('High') },
                  ]}
                  value={values.effort}
                  onValueChange={(value) =>
                    form.setValue(
                      'effort',
                      (value ?? '') as EvaluationValues['effort']
                    )
                  }
                >
                  <SelectTrigger id='evaluation-effort' className='w-full'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value=''>{t('Model default')}</SelectItem>
                    <SelectItem value='low'>{t('Low')}</SelectItem>
                    <SelectItem value='medium'>{t('Medium')}</SelectItem>
                    <SelectItem value='high'>{t('High')}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <p className='text-muted-foreground text-xs'>
                {t('Only select a reasoning effort supported by this model.')}
              </p>
            </div>
          </details>
          <p className='text-muted-foreground text-xs'>
            {t('One model request per test. No automatic retries.')}
          </p>
          <Label
            htmlFor='evaluation-charge'
            className='items-start gap-3 rounded-lg border p-3 leading-relaxed'
          >
            <Checkbox
              id='evaluation-charge'
              className='mt-0.5'
              checked={values.confirm_charge}
              onCheckedChange={(checked) =>
                form.setValue('confirm_charge', checked === true)
              }
            />
            <span>
              {t(
                'I understand this test uses my selected API key and consumes my quota at the normal site rate.'
              )}
            </span>
          </Label>
          {Object.keys(form.formState.errors).length > 0 && (
            <p role='alert' className='text-destructive text-sm'>
              {t('Select a valid API key and model, then confirm the charge.')}
            </p>
          )}
          {start.isError && (
            <p role='alert' className='text-destructive text-sm'>
              {t(
                'Could not start the test. Check the error message and refresh history before retrying.'
              )}
            </p>
          )}
          <Button
            type='submit'
            className='h-10 w-full'
            disabled={
              start.isPending ||
              !token ||
              !tokenAvailable(token) ||
              !values.model ||
              !models.data?.includes(values.model) ||
              !values.confirm_charge ||
              !question
            }
          >
            {start.isPending ? (
              <Loader2
                className='size-4 motion-safe:animate-spin'
                aria-hidden
              />
            ) : (
              <ArrowRight className='size-4' aria-hidden />
            )}
            {start.isPending ? t('Starting test...') : t('Start model test')}
          </Button>
        </CardContent>
      </Card>
    </form>
  )
}
