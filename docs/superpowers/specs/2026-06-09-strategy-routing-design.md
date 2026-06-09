# Strategy Routing Design Spec

## Overview

Add dynamic routing strategy capabilities to the AI API gateway. The current routing system is static (priority + weight + group). This design adds two strategy types:

1. **Difficulty Strategy** - LLM pre-classification routes requests to appropriate models based on query complexity
2. **Time Strategy** - Cron-based scheduling enables/disables models and adjusts channel priority/weight

## Requirements Summary

| Dimension | Decision |
|-----------|----------|
| Difficulty judgment | LLM pre-classification, 3 fixed levels (simple/medium/hard) |
| Time strategy | Both model switching and priority/weight adjustment, cron expressions |
| Scope | Global strategy |
| Classifier | Support reusing existing channels or independent configuration |
| Trigger | All requests auto-evaluate |
| Failure fallback | Degrade to default routing |

## Architecture

### Approach: Middleware Injection

Insert a `StrategyMiddleware` into the relay pipeline before `Distribute`:

```
TokenAuth → ModelRequestRateLimit → StrategyMiddleware → Distribute → Relay
```

The strategy middleware evaluates strategies and modifies routing context. The `Distribute` middleware then selects channels based on the modified context.

## Data Model

### `strategies` Table

```go
type Strategy struct {
    Id          int    `json:"id" gorm:"primaryKey"`
    Name        string `json:"name" gorm:"size:128;not null"`
    Type        string `json:"type" gorm:"size:32;not null"` // "difficulty" / "time"
    Enabled     bool   `json:"enabled" gorm:"default:true"`
    Priority    int    `json:"priority" gorm:"default:0"` // Evaluation order, higher = first

    // Difficulty strategy fields (type="difficulty")
    ClassifierType      string `json:"classifier_type" gorm:"size:32"` // "channel" / "independent"
    ClassifierChannelId int    `json:"classifier_channel_id"`          // Reused channel ID
    ClassifierModel     string `json:"classifier_model" gorm:"size:128"`
    ClassifierApiKey    string `json:"classifier_api_key" gorm:"size:512"`
    ClassifierBaseUrl   string `json:"classifier_base_url" gorm:"size:512"`
    ClassifierPrompt    string `json:"classifier_prompt" gorm:"type:text"`
    ClassifierTimeout   int    `json:"classifier_timeout" gorm:"default:3000"` // ms

    // Difficulty -> model mapping (JSON)
    // {"simple": ["gpt-4o-mini"], "medium": ["gpt-4o"], "hard": ["gpt-4o", "claude-3.5-sonnet"]}
    DifficultyModels string `json:"difficulty_models" gorm:"type:text"`

    // Time strategy fields (type="time")
    CronExpr string `json:"cron_expr" gorm:"size:128"`
    Timezone string `json:"timezone" gorm:"size:64"`

    // Time strategy actions (JSON)
    // {"enable_models": [...], "disable_models": [...],
    //  "priority_adjust": {"channel_id": +10}, "weight_adjust": {"channel_id": *2},
    //  "use_models": [...]}
    TimeActions string `json:"time_actions" gorm:"type:text"`

    Description string `json:"description" gorm:"size:512"`
    CreatedAt   int64  `json:"created_at"`
    UpdatedAt   int64  `json:"updated_at"`
}
```

### `strategy_logs` Table

```go
type StrategyLog struct {
    Id         int    `gorm:"primaryKey"`
    StrategyId int    `gorm:"index"`
    RequestId  string `gorm:"size:64"`
    Model      string `gorm:"size:128"`
    Result     string `gorm:"size:32"` // "simple"/"medium"/"hard"/"fallback"/"time_match"
    LatencyMs  int
    Error      string `gorm:"size:512"`
    CreatedAt  int64
}
```

### Difficulty Level -> Model Mapping Example

```json
{
  "simple": ["gpt-4o-mini", "gemini-2.0-flash"],
  "medium": ["gpt-4o", "claude-3.5-haiku"],
  "hard": ["gpt-4o", "claude-3.5-sonnet", "o1"]
}
```

## Strategy Middleware

### Pipeline Position

```
TokenAuth → ModelRequestRateLimit → StrategyMiddleware → Distribute → Relay
```

### Middleware Flow

```
1. Load all enabled strategies from cache (refreshed every 30s)
2. Sort by Priority descending
3. For each strategy:
   a. type="time":
      - Evaluate cron expression against current time
      - If match: execute TimeActions (enable/disable models, adjust priority/weight)
      - Write adjusted model list / channel visibility to context
   b. type="difficulty":
      - Check result cache (hash of first 200 chars of user message, TTL 5min)
      - If cache miss:
        - Build classification request with user messages
        - Call classifier LLM (timeout 3s, fallback to default on failure)
        - Parse result (simple/medium/hard)
        - Cache result
      - Set StrategyDifficultyLevel in context
      - Set StrategyModels in context (from DifficultyModels mapping)
4. Distribute middleware reads context and selects channel accordingly
```

### LLM Classifier

Default prompt:

```
You are a request complexity classifier. Based on the user's request content, determine its complexity level.

Classification criteria:
- simple: Simple Q&A, translation, formatting, short text processing
- medium: Code generation, text analysis, medium-length reasoning tasks
- hard: Complex math derivation, multi-step programming, long document analysis, system design

Return only JSON: {"level": "simple|medium|hard", "reason": "brief reason"}
```

The prompt is customizable per strategy via `ClassifierPrompt` field.

### Classifier Configuration

| Field | Description |
|-------|-------------|
| ClassifierType | "channel" (reuse existing) or "independent" |
| ClassifierChannelId | Channel ID to use (when type="channel") |
| ClassifierModel | Model name for classification |
| ClassifierApiKey | API key (when type="independent") |
| ClassifierBaseUrl | Base URL (when type="independent") |
| ClassifierTimeout | Timeout in ms (default 3000) |

### Time Strategy Actions

```go
type TimeActions struct {
    EnableModels   []string       `json:"enable_models,omitempty"`
    DisableModels  []string       `json:"disable_models,omitempty"`
    PriorityAdjust map[string]int `json:"priority_adjust,omitempty"` // channelId (as string) -> priority delta (e.g., +10, -5)
    WeightAdjust   map[string]int `json:"weight_adjust,omitempty"`   // channelId (as string) -> weight delta (e.g., +20, -5)
    UseModels      []string       `json:"use_models,omitempty"`      // Replace with specific models
}
```

Note: `PriorityAdjust` and `WeightAdjust` use additive deltas, not multipliers. Channel IDs are stored as string keys (e.g., `{"3": 10}` adds 10 to channel 3's priority).

## Edge Cases

| Scenario | Handling |
|----------|----------|
| Classifier timeout (3s) | Fallback to default routing, log warning |
| Classifier returns invalid format | Fallback to default routing, log warning |
| No strategies configured | Skip middleware, default routing |
| Multiple difficulty strategies | Execute only the first enabled difficulty strategy (by priority descending) |
| Multiple time strategies | All matching time strategies execute in priority order; their actions accumulate |
| Difficulty + Time coexist | Both types execute: difficulty strategy determines candidate model list, time strategy may further mask/adjust channels. Time actions apply after difficulty mapping |
| Cache hit but mapped models unavailable | Cache result still valid, Distribute picks from available models in list |
| Invalid cron expression | Strategy skipped, log error |

## Performance

| Concern | Mitigation |
|---------|------------|
| Classification latency | Small model classification ~200-500ms, 3s timeout cap |
| Cache hit | Same request within 5min returns cached result, zero latency |
| Strategy loading | In-memory cache + 30s refresh, no DB query per request |
| Time strategy | Cron check is pure CPU, near-zero overhead |

## API Endpoints

```
GET    /api/strategy/           - List all strategies
GET    /api/strategy/:id        - Get single strategy
POST   /api/strategy/           - Create strategy
PUT    /api/strategy/           - Update strategy
DELETE /api/strategy/:id        - Delete strategy
POST   /api/strategy/test       - Test classifier (send test request, return classification result)
GET    /api/strategy/logs       - Get strategy execution logs (paginated)
```

## Frontend UI

### Location

System Settings -> Models & Routing -> Strategy (`/system-settings/models/strategy`)

### Page Layout

- Strategy list with two sections: Difficulty Strategies and Time Strategies
- Each strategy card shows: name, status, priority, configuration summary, action buttons (edit, test, delete)
- Strategy logs table at the bottom

### Edit Dialogs

**Difficulty Strategy Dialog:**
- Name, priority, status
- Classifier config: type (channel/independent), channel selector, model selector, timeout, custom prompt
- Difficulty -> model mapping: three multi-select fields (simple, medium, hard)

**Time Strategy Dialog:**
- Name, priority, status
- Cron expression input with timezone selector and human-readable explanation
- Action checkboxes: enable/disable models, priority adjustment, weight adjustment

## Implementation Order

1. Database migration (strategies + strategy_logs tables)
2. Strategy model + CRUD service
3. Strategy API endpoints (controller + router)
4. LLM classifier service
5. Strategy middleware
6. Strategy cache (in-memory + refresh)
7. Frontend: strategy management page
8. Frontend: difficulty strategy edit dialog
9. Frontend: time strategy edit dialog
10. Frontend: strategy test dialog
11. Frontend: strategy logs view
12. Integration testing
