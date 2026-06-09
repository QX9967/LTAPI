import { z } from 'zod'

export const strategySchema = z.object({
  id: z.number(),
  name: z.string(),
  type: z.enum(['difficulty', 'time']),
  enabled: z.boolean(),
  priority: z.number(),
  classifier_type: z.enum(['channel', 'independent']).optional(),
  classifier_channel_id: z.number().optional(),
  classifier_model: z.string().optional(),
  classifier_api_key: z.string().optional(),
  classifier_base_url: z.string().optional(),
  classifier_prompt: z.string().optional(),
  classifier_timeout: z.number().default(3000),
  difficulty_models: z.string().optional(),
  cron_expr: z.string().optional(),
  timezone: z.string().optional(),
  time_actions: z.string().optional(),
  description: z.string().optional(),
  created_at: z.number(),
  updated_at: z.number(),
})

export type Strategy = z.infer<typeof strategySchema>

export const difficultyModelsSchema = z.object({
  simple: z.array(z.string()),
  medium: z.array(z.string()),
  hard: z.array(z.string()),
})

export type DifficultyModels = z.infer<typeof difficultyModelsSchema>

export const timeActionsSchema = z.object({
  enable_models: z.array(z.string()).optional(),
  disable_models: z.array(z.string()).optional(),
  priority_adjust: z.record(z.number()).optional(),
  weight_adjust: z.record(z.number()).optional(),
  use_models: z.array(z.string()).optional(),
})

export type TimeActions = z.infer<typeof timeActionsSchema>

export const classifierResultSchema = z.object({
  level: z.enum(['simple', 'medium', 'hard']),
  reason: z.string(),
})

export type ClassifierResult = z.infer<typeof classifierResultSchema>

export interface StrategyLog {
  id: number
  strategy_id: number
  request_id: string
  model: string
  result: string
  latency_ms: number
  error: string
  created_at: number
}
