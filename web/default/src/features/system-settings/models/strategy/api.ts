import { api } from '@/lib/api'
import type { Strategy, ClassifierResult, StrategyLog } from './types'

export async function getStrategies() {
  const res = await api.get('/api/strategy/')
  return res.data as { success: boolean; data: Strategy[] }
}

export async function getStrategy(id: number) {
  const res = await api.get(`/api/strategy/${id}`)
  return res.data as { success: boolean; data: Strategy }
}

export async function createStrategy(strategy: Partial<Strategy>) {
  const res = await api.post('/api/strategy/', strategy)
  return res.data as { success: boolean; data: Strategy }
}

export async function updateStrategy(strategy: Partial<Strategy>) {
  const res = await api.put('/api/strategy/', strategy)
  return res.data as { success: boolean; data: Strategy }
}

export async function deleteStrategy(id: number) {
  const res = await api.delete(`/api/strategy/${id}`)
  return res.data as { success: boolean }
}

export async function testClassifier(params: {
  classifier_type: string
  classifier_channel_id?: number
  classifier_model?: string
  classifier_api_key?: string
  classifier_base_url?: string
  classifier_prompt?: string
  classifier_timeout?: number
  test_message: string
}) {
  const res = await api.post('/api/strategy/test', params, {
    skipBusinessError: true,
    skipErrorHandler: true,
  } as any)
  return res.data as {
    success: boolean
    message?: string
    data?: ClassifierResult
  }
}

export async function getStrategyLogs(params: {
  strategy_id?: number
  p?: number
  size?: number
}) {
  const res = await api.get('/api/strategy/logs', { params })
  return res.data as {
    success: boolean
    data: StrategyLog[]
    total: number
  }
}
