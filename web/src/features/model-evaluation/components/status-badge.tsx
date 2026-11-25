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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

import type { EvaluationStatus } from '../types'

export function EvaluationStatusBadge(props: { status: EvaluationStatus }) {
  const { t } = useTranslation()
  const labels: Record<EvaluationStatus, string> = {
    running: t('Evaluation running'),
    passed: t('Test passed'),
    failed: t('Test not passed'),
    error: t('Request failed'),
    truncated: t('Output truncated'),
    review: t('Awaiting your review'),
    partial: t('Partially meets requirements'),
  }
  return (
    <Badge
      variant='outline'
      className={cn(
        'gap-1.5 whitespace-nowrap',
        props.status === 'passed' &&
          'border-primary/30 bg-primary/10 text-primary',
        (props.status === 'failed' || props.status === 'error') &&
          'border-destructive/30 bg-destructive/10 text-destructive'
      )}
    >
      {props.status === 'running' && (
        <Loader2 className='size-3 motion-safe:animate-spin' aria-hidden />
      )}
      {labels[props.status]}
    </Badge>
  )
}
