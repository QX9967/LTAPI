export const DIFFICULTY_LEVELS = ['simple', 'medium', 'hard'] as const

export const DIFFICULTY_LEVEL_LABELS: Record<string, string> = {
  simple: 'Simple',
  medium: 'Medium',
  hard: 'Hard',
}

export const CLASSIFIER_TYPES = ['channel', 'independent'] as const

export const DEFAULT_DIFFICULTY_MODELS = {
  simple: [],
  medium: [],
  hard: [],
}

export const DEFAULT_TIME_ACTIONS = {
  enable_models: [],
  disable_models: [],
  priority_adjust: {},
  weight_adjust: {},
  use_models: [],
}

export const COMMON_TIMEZONES = [
  'UTC',
  'Asia/Shanghai',
  'Asia/Tokyo',
  'America/New_York',
  'America/Los_Angeles',
  'Europe/London',
  'Europe/Berlin',
]
