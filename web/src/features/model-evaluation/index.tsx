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
import { useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { ClipboardCheck, History } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useAuthStore } from '@/stores/auth-store'

import { EvaluationForm } from './components/evaluation-form'
import { EvaluationHistory } from './components/evaluation-history'
import { EvaluationReport } from './components/evaluation-report'
import type { EvaluationRun } from './types'

export function ModelEvaluation() {
  const { t } = useTranslation()
  const search = useSearch({ from: '/_authenticated/model-test/' })
  const navigate = useNavigate({ from: '/model-test/' })
  const queryClient = useQueryClient()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const [source, setSource] = useState<EvaluationRun>()

  function repeat(run: EvaluationRun): void {
    setSource(run)
    void navigate({ search: { tab: 'new', page: 1 } })
  }

  function started(id: number): void {
    void queryClient.invalidateQueries({ queryKey: ['evaluations', userId] })
    void navigate({ search: { tab: 'history', run: id, page: 1 } })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Model testing')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Badge variant='outline'>{t('Site models only')}</Badge>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='w-full min-w-0 space-y-5'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Test real model responses with your own API key, inspect the evidence and compare your history.'
            )}
          </p>
          <Tabs
            value={search.tab}
            onValueChange={(tab) => {
              void navigate({
                search: {
                  ...search,
                  tab: tab === 'history' ? 'history' : 'new',
                },
              })
            }}
          >
            <TabsList className='mb-3 h-10'>
              <TabsTrigger value='new' className='px-4'>
                <ClipboardCheck className='size-4' aria-hidden />
                {t('New test')}
              </TabsTrigger>
              <TabsTrigger value='history' className='px-4'>
                <History className='size-4' aria-hidden />
                {t('My history')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value='new'>
              <EvaluationForm
                key={source?.id ?? 'new'}
                source={source}
                onStarted={started}
              />
            </TabsContent>
            <TabsContent
              value='history'
              className={
                search.run
                  ? 'grid items-start gap-4 2xl:grid-cols-[18rem_minmax(0,1fr)]'
                  : 'space-y-4'
              }
            >
              <div className='min-w-0 2xl:sticky 2xl:top-0'>
                <EvaluationHistory
                  page={search.page}
                  selected={search.run}
                  onPage={(page) => {
                    void navigate({ search: { ...search, page } })
                  }}
                />
              </div>
              {search.run && (
                <div className='order-first min-w-0 2xl:order-none'>
                  <EvaluationReport
                    key={search.run}
                    id={search.run}
                    onRepeat={repeat}
                  />
                </div>
              )}
            </TabsContent>
          </Tabs>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
