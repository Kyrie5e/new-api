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
export type EvaluationStatus =
  | 'running'
  | 'passed'
  | 'failed'
  | 'error'
  | 'truncated'
  | 'review'
  | 'partial'

export type EvaluationQuestion = {
  id: string
  name: string
  description: string
  kind: 'text' | 'artwork'
  version: string
  max_output: number
}

export type EvaluationRun = {
  id: number
  token_id: number
  token_name: string
  token_masked: string
  token_group: string
  model: string
  protocol: 'chat' | 'responses'
  effort: string
  question_id: string
  question_name: string
  question_version: string
  kind: 'text' | 'artwork'
  max_output: number
  prompt: string
  expected: string
  answer: string
  artwork: string
  status: EvaluationStatus
  error: string
  finish_reason: string
  request_id: string
  prompt_tokens: number
  completion_tokens: number
  quota: number | null
  created_at: number
  completed_at: number
  duration_ms: number
}

export type StartEvaluation = {
  token_id: number
  model: string
  question_id: string
  protocol: 'chat' | 'responses'
  effort: string
  confirm_charge: boolean
  source_id?: number
}

export type EvaluationHistory = {
  items: EvaluationRun[]
  total: number
  page: number
  page_size: number
}
