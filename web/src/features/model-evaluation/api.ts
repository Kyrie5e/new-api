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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  EvaluationHistory,
  EvaluationQuestion,
  EvaluationRun,
  StartEvaluation,
} from './types'

function evaluationData<T>(response: {
  success: boolean
  message?: string
  data: T
}): T {
  return requireServerSuccess(response).data
}

export async function getEvaluationQuestions(): Promise<EvaluationQuestion[]> {
  const response = await api.get('/api/evaluations/questions')
  return evaluationData(response.data)
}

export async function getEvaluationModels(tokenId: number): Promise<string[]> {
  const response = await api.get('/api/evaluations/models', {
    params: { token_id: tokenId },
  })
  return evaluationData(response.data)
}

export async function startEvaluation(
  input: StartEvaluation
): Promise<{ id: number }> {
  const response = await api.post('/api/evaluations/runs', input)
  return evaluationData(response.data)
}

export async function getEvaluationHistory(
  page: number
): Promise<EvaluationHistory> {
  const response = await api.get('/api/evaluations/runs', {
    params: { p: page, size: 10 },
  })
  return evaluationData(response.data)
}

export async function getEvaluationReport(id: number): Promise<EvaluationRun> {
  const response = await api.get(`/api/evaluations/runs/${id}`)
  return evaluationData(response.data)
}

export async function rateEvaluation(
  id: number,
  status: string
): Promise<void> {
  const response = await api.post(`/api/evaluations/runs/${id}/rating`, {
    status,
  })
  evaluationData(response.data)
}
