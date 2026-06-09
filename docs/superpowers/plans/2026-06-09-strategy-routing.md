# Strategy Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add dynamic routing strategy capabilities — LLM-based difficulty classification and cron-based time scheduling — to the AI API gateway.

**Architecture:** Insert a `StrategyMiddleware` into the relay pipeline before `Distribute`. The middleware evaluates difficulty strategies (LLM pre-classification) and time strategies (cron scheduling), then modifies routing context so `Distribute` selects channels accordingly. Strategy configurations are stored in a new `strategies` database table with in-memory caching.

**Tech Stack:** Go, Gin, GORM, robfig/cron/v3, react-hook-form, zod, @tanstack/react-table

---

## File Structure

### Backend (Go)

| File | Responsibility |
|------|---------------|
| `model/strategy.go` | Strategy + StrategyLog GORM models, DB CRUD |
| `model/strategy_cache.go` | In-memory strategy cache with 30s refresh |
| `service/strategy.go` | Strategy business logic: evaluation, LLM classification |
| `service/strategy_classifier.go` | LLM classifier: builds request, calls classifier model, parses result |
| `service/strategy_time.go` | Time strategy evaluation: cron matching, action execution |
| `middleware/strategy.go` | StrategyMiddleware: loads strategies, evaluates, sets context |
| `controller/strategy.go` | Strategy CRUD API handlers |
| `constant/context_key.go` | Add strategy context keys |
| `router/api-router.go` | Register strategy API routes |
| `router/relay-router.go` | Insert StrategyMiddleware before Distribute |
| `model/main.go` | Add Strategy + StrategyLog to AutoMigrate |

### Frontend (React/TypeScript)

| File | Responsibility |
|------|---------------|
| `web/default/src/features/system-settings/models/strategy/` | Strategy management section |
| `web/default/src/features/system-settings/models/strategy/index.tsx` | Main strategy section component |
| `web/default/src/features/system-settings/models/strategy/api.ts` | Strategy API calls |
| `web/default/src/features/system-settings/models/strategy/types.ts` | TypeScript types + Zod schemas |
| `web/default/src/features/system-settings/models/strategy/constants.ts` | Strategy constants |
| `web/default/src/features/system-settings/models/strategy/difficulty-strategy-dialog.tsx` | Difficulty strategy edit dialog |
| `web/default/src/features/system-settings/models/strategy/time-strategy-dialog.tsx` | Time strategy edit dialog |
| `web/default/src/features/system-settings/models/strategy/strategy-test-dialog.tsx` | Classifier test dialog |
| `web/default/src/features/system-settings/models/strategy/strategy-logs-table.tsx` | Strategy logs table |
| `web/default/src/features/system-settings/models/section-registry.tsx` | Add strategy section |
| `web/default/src/components/layout/config/system-settings.config.ts` | Add strategy nav item |

---

## Task 1: Database Models and Migration

**Files:**
- Create: `model/strategy.go`
- Modify: `model/main.go:263-289`

- [ ] **Step 1: Create Strategy and StrategyLog models**

```go
// model/strategy.go
package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

type Strategy struct {
	Id       int    `json:"id" gorm:"primaryKey"`
	Name     string `json:"name" gorm:"size:128;not null"`
	Type     string `json:"type" gorm:"size:32;not null;index"`
	Enabled  bool   `json:"enabled" gorm:"default:true;index"`
	Priority int    `json:"priority" gorm:"default:0;index"`

	// Difficulty strategy fields
	ClassifierType      string `json:"classifier_type" gorm:"size:32"`
	ClassifierChannelId int    `json:"classifier_channel_id"`
	ClassifierModel     string `json:"classifier_model" gorm:"size:128"`
	ClassifierApiKey    string `json:"classifier_api_key" gorm:"size:512"`
	ClassifierBaseUrl   string `json:"classifier_base_url" gorm:"size:512"`
	ClassifierPrompt    string `json:"classifier_prompt" gorm:"type:text"`
	ClassifierTimeout   int    `json:"classifier_timeout" gorm:"default:3000"`
	DifficultyModels    string `json:"difficulty_models" gorm:"type:text"`

	// Time strategy fields
	CronExpr    string `json:"cron_expr" gorm:"size:128"`
	Timezone    string `json:"timezone" gorm:"size:64"`
	TimeActions string `json:"time_actions" gorm:"type:text"`

	Description string `json:"description" gorm:"size:512"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

func (s *Strategy) BeforeCreate(tx *DB) error {
	if s.CreatedAt == 0 {
		s.CreatedAt = time.Now().Unix()
	}
	if s.UpdatedAt == 0 {
		s.UpdatedAt = time.Now().Unix()
	}
	return nil
}

func (s *Strategy) BeforeUpdate(tx *DB) error {
	s.UpdatedAt = time.Now().Unix()
	return nil
}

func (s *Strategy) TableName() string {
	return "strategies"
}

type StrategyLog struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	StrategyId int    `json:"strategy_id" gorm:"index"`
	RequestId  string `json:"request_id" gorm:"size:64;index"`
	Model      string `json:"model" gorm:"size:128"`
	Result     string `json:"result" gorm:"size:32"`
	LatencyMs  int    `json:"latency_ms"`
	Error      string `json:"error" gorm:"size:512"`
	CreatedAt  int64  `json:"created_at"`
}

func (l *StrategyLog) BeforeCreate(tx *DB) error {
	if l.CreatedAt == 0 {
		l.CreatedAt = time.Now().Unix()
	}
	return nil
}

func (l *StrategyLog) TableName() string {
	return "strategy_logs"
}

func GetAllEnabledStrategies() ([]Strategy, error) {
	var strategies []Strategy
	err := DB.Where("enabled = ?", true).Order("priority DESC").Find(&strategies).Error
	return strategies, err
}

func GetStrategyById(id int) (*Strategy, error) {
	var strategy Strategy
	err := DB.Where("id = ?", id).First(&strategy).Error
	if err != nil {
		return nil, err
	}
	return &strategy, nil
}

func GetAllStrategies() ([]Strategy, error) {
	var strategies []Strategy
	err := DB.Order("priority DESC").Find(&strategies).Error
	return strategies, err
}

func (s *Strategy) Insert() error {
	return DB.Create(s).Error
}

func (s *Strategy) Update() error {
	return DB.Save(s).Error
}

func (s *Strategy) Delete() error {
	return DB.Delete(s).Error
}

func CreateStrategyLog(log *StrategyLog) error {
	return DB.Create(log).Error
}

func GetStrategyLogs(strategyId int, page, pageSize int) ([]StrategyLog, int64, error) {
	var logs []StrategyLog
	var total int64
	query := DB.Model(&StrategyLog{})
	if strategyId > 0 {
		query = query.Where("strategy_id = ?", strategyId)
	}
	query.Count(&total)
	err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}

func DeleteOldStrategyLogs(beforeTimestamp int64) error {
	return DB.Where("created_at < ?", beforeTimestamp).Delete(&StrategyLog{}).Error
}
```

- [ ] **Step 2: Add models to DB migration**

In `model/main.go`, add `&Strategy{}` and `&StrategyLog{}` to the `DB.AutoMigrate()` call:

```go
err := DB.AutoMigrate(
    // ... existing models ...
    &PerfMetric{},
    &Strategy{},      // ADD THIS
    &StrategyLog{},   // ADD THIS
)
```

- [ ] **Step 3: Run migration and verify tables created**

```bash
cd D:\Project\Adayo\new-api
go run main.go
# Check logs for "database migration completed" or similar
# Verify strategies and strategy_logs tables exist in DB
```

- [ ] **Step 4: Commit**

```bash
git add model/strategy.go model/main.go
git commit -m "feat(strategy): add Strategy and StrategyLog models with DB migration"
```

---

## Task 2: Strategy Cache

**Files:**
- Create: `model/strategy_cache.go`

- [ ] **Step 1: Create strategy cache**

```go
// model/strategy_cache.go
package model

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var (
	strategiesCache     []Strategy
	strategiesCacheLock sync.RWMutex
	strategiesCacheTime time.Time
	strategiesCacheTTL  = 30 * time.Second
)

func InitStrategyCache() {
	strategies, err := GetAllEnabledStrategies()
	if err != nil {
		common.SysLog("failed to init strategy cache: " + err.Error())
		return
	}
	strategiesCacheLock.Lock()
	strategiesCache = strategies
	strategiesCacheTime = time.Now()
	strategiesCacheLock.Unlock()
}

func GetCachedStrategies() []Strategy {
	strategiesCacheLock.Lock()
	defer strategiesCacheLock.Unlock()

	if time.Since(strategiesCacheTime) > strategiesCacheTTL {
		strategies, err := GetAllEnabledStrategies()
		if err == nil {
			strategiesCache = strategies
			strategiesCacheTime = time.Now()
		}
	}

	result := make([]Strategy, len(strategiesCache))
	copy(result, strategiesCache)
	return result
}

func RefreshStrategyCache() {
	InitStrategyCache()
}
```

- [ ] **Step 2: Initialize cache on startup**

In `model/main.go`, after `migrateDB()` succeeds, call `InitStrategyCache()`.

- [ ] **Step 3: Commit**

```bash
git add model/strategy_cache.go model/main.go
git commit -m "feat(strategy): add in-memory strategy cache with 30s refresh"
```

---

## Task 3: Strategy Context Keys

**Files:**
- Modify: `constant/context_key.go`

- [ ] **Step 1: Add strategy context keys**

Add these constants to the `ContextKey` block in `constant/context_key.go`:

```go
// Strategy keys
ContextKeyStrategyModels         ContextKey = "strategy_models"
ContextKeyStrategyDifficultyLevel ContextKey = "strategy_difficulty_level"
ContextKeyStrategyTimeMasked     ContextKey = "strategy_time_masked"
```

- [ ] **Step 2: Commit**

```bash
git add constant/context_key.go
git commit -m "feat(strategy): add strategy context keys"
```

---

## Task 4: LLM Classifier Service

**Files:**
- Create: `service/strategy_classifier.go`

- [ ] **Step 1: Create classifier service**

```go
// service/strategy_classifier.go
package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const defaultClassifierPrompt = `You are a request complexity classifier. Based on the user's request content, determine its complexity level.

Classification criteria:
- simple: Simple Q&A, translation, formatting, short text processing, greetings
- medium: Code generation, text analysis, medium-length reasoning tasks, explanations
- hard: Complex math derivation, multi-step programming, long document analysis, system design, architecture

Return only JSON: {"level": "simple|medium|hard", "reason": "brief reason"}`

type ClassifierResult struct {
	Level  string `json:"level"`
	Reason string `json:"reason"`
}

func ClassifyDifficulty(strategyId int, classifierType string, channelId int, modelName, apiKey, baseUrl, customPrompt string, timeout int, userMessages []map[string]string) (*ClassifierResult, error) {
	start := time.Now()

	prompt := defaultClassifierPrompt
	if customPrompt != "" {
		prompt = customPrompt
	}

	// Build the classification request
	classifyMessages := buildClassifyMessages(prompt, userMessages)

	var result *ClassifierResult
	var classifyErr error

	if classifierType == "channel" && channelId > 0 {
		result, classifyErr = classifyViaChannel(channelId, modelName, classifyMessages, timeout)
	} else if classifierType == "independent" && apiKey != "" {
		result, classifyErr = classifyViaIndependent(apiKey, baseUrl, modelName, classifyMessages, timeout)
	} else {
		return nil, fmt.Errorf("invalid classifier configuration")
	}

	latencyMs := int(time.Since(start).Milliseconds())

	// Log the classification
	go func() {
		log := &model.StrategyLog{
			StrategyId: strategyId,
			LatencyMs:  latencyMs,
		}
		if result != nil {
			log.Result = result.Level
		}
		if classifyErr != nil {
			log.Error = classifyErr.Error()
			log.Result = "fallback"
		}
		model.CreateStrategyLog(log)
	}()

	if classifyErr != nil {
		return nil, classifyErr
	}

	// Validate result
	if result.Level != "simple" && result.Level != "medium" && result.Level != "hard" {
		return nil, fmt.Errorf("invalid classification level: %s", result.Level)
	}

	return result, nil
}

func buildClassifyMessages(systemPrompt string, userMessages []map[string]string) []map[string]string {
	messages := []map[string]string{
		{"role": "system", "content": systemPrompt},
	}
	// Include last few user messages for context (max 3)
	count := 0
	for i := len(userMessages) - 1; i >= 0 && count < 3; i-- {
		if userMessages[i]["role"] == "user" {
			content := userMessages[i]["content"]
			// Truncate long messages
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			messages = append([]map[string]string{
				{"role": "user", "content": content},
			}, messages[1:]...)
			count++
		}
	}
	return messages
}

func classifyViaChannel(channelId int, modelName string, messages []map[string]string, timeout int) (*ClassifierResult, error) {
	channel, err := model.CacheGetChannel(channelId)
	if err != nil || channel == nil {
		return nil, fmt.Errorf("classifier channel not found: %d", channelId)
	}

	// Use the channel's key and base URL
	key := channel.Key
	baseUrl := ""
	if channel.BaseURL != nil {
		baseUrl = *channel.BaseURL
	}

	return classifyViaIndependent(key, baseUrl, modelName, messages, timeout)
}

func classifyViaIndependent(apiKey, baseUrl, modelName string, messages []map[string]string, timeout int) (*ClassifierResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
	defer cancel()

	_ = ctx // Use ctx for HTTP request timeout

	// Build OpenAI-compatible request
	requestBody := map[string]interface{}{
		"model":       modelName,
		"messages":    messages,
		"max_tokens":  100,
		"temperature": 0,
	}

	jsonBytes, err := common.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(baseUrl, "/") + "/v1/chat/completions"
	if baseUrl == "" {
		url = "https://api.openai.com/v1/chat/completions"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: time.Duration(timeout) * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("classifier request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("classifier returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse OpenAI response
	type Choice struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	type Response struct {
		Choices []Choice `json:"choices"`
	}

	var respData Response
	if err := common.Unmarshal(body, &respData); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(respData.Choices) == 0 {
		return nil, fmt.Errorf("no choices in classifier response")
	}

	content := respData.Choices[0].Message.Content

	// Parse JSON from content
	var result ClassifierResult
	if err := common.Unmarshal([]byte(content), &result); err != nil {
		// Try to extract level from plain text
		content = strings.TrimSpace(content)
		if strings.Contains(content, "simple") {
			result.Level = "simple"
		} else if strings.Contains(content, "hard") {
			result.Level = "hard"
		} else {
			result.Level = "medium"
		}
	}

	return &result, nil
}
```

Note: This file will need `import "net/http"` and `import "io"` added to the imports. The actual implementation should use the project's existing HTTP client patterns.

- [ ] **Step 2: Verify compilation**

```bash
go build ./service/
```

- [ ] **Step 3: Commit**

```bash
git add service/strategy_classifier.go
git commit -m "feat(strategy): add LLM classifier service for difficulty classification"
```

---

## Task 5: Strategy Middleware

**Files:**
- Create: `middleware/strategy.go`
- Modify: `router/relay-router.go`

- [ ] **Step 1: Create strategy middleware**

```go
// middleware/strategy.go
package middleware

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
)

var (
	cronParser  = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	classifyCache     = make(map[string]classifyCacheEntry)
	classifyCacheLock sync.RWMutex
)

type classifyCacheEntry struct {
	level     string
	expiresAt time.Time
}

func StrategyMiddleware() func(c *gin.Context) {
	return func(c *gin.Context) {
		strategies := model.GetCachedStrategies()
		if len(strategies) == 0 {
			c.Next()
			return
		}

		var strategyModels []string
		var difficultyLevel string
		maskedChannels := make(map[int]bool)

		for _, strategy := range strategies {
			if !strategy.Enabled {
				continue
			}

			switch strategy.Type {
			case "difficulty":
				if strategyModels != nil {
					continue // Already classified by higher-priority strategy
				}
				level, models, err := evaluateDifficultyStrategy(c, &strategy)
				if err != nil {
					// Fallback to default routing
					continue
				}
				difficultyLevel = level
				strategyModels = models

			case "time":
				actions, err := evaluateTimeStrategy(&strategy)
				if err != nil || actions == nil {
					continue
				}
				// Apply time actions
				if len(actions.UseModels) > 0 {
					strategyModels = actions.UseModels
				}
				for _, m := range actions.DisableModels {
					maskedChannels[m] = true
				}
			}
		}

		// Set context keys for Distribute to read
		if len(strategyModels) > 0 {
			common.SetContextKey(c, constant.ContextKeyStrategyModels, strategyModels)
		}
		if difficultyLevel != "" {
			common.SetContextKey(c, constant.ContextKeyStrategyDifficultyLevel, difficultyLevel)
		}

		c.Next()
	}
}

func evaluateDifficultyStrategy(c *gin.Context, strategy *model.Strategy) (string, []string, error) {
	// Extract user messages from request body
	messages, err := extractUserMessages(c)
	if err != nil || len(messages) == 0 {
		return "", nil, fmt.Errorf("no user messages found")
	}

	// Check cache
	cacheKey := computeClassifyCacheKey(messages)
	if cached, ok := getCachedClassification(cacheKey); ok {
		models := getModelsForLevel(strategy.DifficultyModels, cached)
		return cached, models, nil
	}

	// Classify via LLM
	result, err := service.ClassifyDifficulty(
		strategy.Id,
		strategy.ClassifierType,
		strategy.ClassifierChannelId,
		strategy.ClassifierModel,
		strategy.ClassifierApiKey,
		strategy.ClassifierBaseUrl,
		strategy.ClassifierPrompt,
		strategy.ClassifierTimeout,
		messages,
	)
	if err != nil {
		return "", nil, err
	}

	// Cache result
	setCachedClassification(cacheKey, result.Level)

	models := getModelsForLevel(strategy.DifficultyModels, result.Level)
	return result.Level, models, nil
}

type TimeActions struct {
	EnableModels   []string       `json:"enable_models,omitempty"`
	DisableModels  []string       `json:"disable_models,omitempty"`
	PriorityAdjust map[string]int `json:"priority_adjust,omitempty"`
	WeightAdjust   map[string]int `json:"weight_adjust,omitempty"`
	UseModels      []string       `json:"use_models,omitempty"`
}

func evaluateTimeStrategy(strategy *model.Strategy) (*TimeActions, error) {
	if strategy.CronExpr == "" {
		return nil, nil
	}

	schedule, err := cronParser.Parse(strategy.CronExpr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	loc := time.UTC
	if strategy.Timezone != "" {
		if l, err := time.LoadLocation(strategy.Timezone); err == nil {
			loc = l
		}
	}

	now := time.Now().In(loc)
	next := schedule.Next(now.Add(-time.Minute))
	if !next.Before(now.Add(time.Minute)) {
		// Not in the current time window
		return nil, nil
	}

	// Parse time actions
	var actions TimeActions
	if strategy.TimeActions != "" {
		if err := common.Unmarshal([]byte(strategy.TimeActions), &actions); err != nil {
			return nil, fmt.Errorf("failed to parse time actions: %w", err)
		}
	}

	return &actions, nil
}

func extractUserMessages(c *gin.Context) ([]map[string]string, error) {
	// Read request body
	body, err := common.UnmarshalBodyReusable(c)
	if err != nil {
		return nil, err
	}

	// Parse messages from various formats
	var messages []map[string]string

	// Try OpenAI format
	if msgs, ok := body["messages"].([]interface{}); ok {
		for _, msg := range msgs {
			if m, ok := msg.(map[string]interface{}); ok {
				role, _ := m["role"].(string)
				content, _ := m["content"].(string)
				if role == "user" && content != "" {
					messages = append(messages, map[string]string{
						"role":    role,
						"content": content,
					})
				}
			}
		}
	}

	return messages, nil
}

func computeClassifyCacheKey(messages []map[string]string) string {
	var sb strings.Builder
	for _, m := range messages {
		if m["role"] == "user" {
			content := m["content"]
			if len(content) > 200 {
				content = content[:200]
			}
			sb.WriteString(content)
		}
	}
	hash := sha256.Sum256([]byte(sb.String()))
	return fmt.Sprintf("%x", hash[:8])
}

func getCachedClassification(key string) (string, bool) {
	classifyCacheLock.RLock()
	defer classifyCacheLock.RUnlock()
	if entry, ok := classifyCache[key]; ok && time.Now().Before(entry.expiresAt) {
		return entry.level, true
	}
	return "", false
}

func setCachedClassification(key, level string) {
	classifyCacheLock.Lock()
	defer classifyCacheLock.Unlock()
	classifyCache[key] = classifyCacheEntry{
		level:     level,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
}

func getModelsForLevel(difficultyModelsJSON string, level string) []string {
	var mapping map[string][]string
	if err := common.Unmarshal([]byte(difficultyModelsJSON), &mapping); err != nil {
		return nil
	}
	return mapping[level]
}
```

Note: This file will need proper imports and the `sync` package. The actual implementation should handle the `UnmarshalBodyReusable` pattern correctly.

- [ ] **Step 2: Insert middleware into relay pipeline**

In `router/relay-router.go`, add `middleware.StrategyMiddleware()` before `middleware.Distribute()` in all relay router groups:

```go
// In compatRouter (line ~74):
compatRouter.Use(middleware.TokenAuth())
compatRouter.Use(middleware.ModelRequestRateLimit())
compatRouter.Use(middleware.StrategyMiddleware())  // ADD THIS
compatRouter.Use(middleware.Distribute())

// In relayV1Router HTTP group (line ~97):
httpRouter.Use(middleware.StrategyMiddleware())  // ADD THIS
httpRouter.Use(middleware.Distribute())

// In relayGeminiRouter (line ~208):
relayGeminiRouter.Use(middleware.StrategyMiddleware())  // ADD THIS
relayGeminiRouter.Use(middleware.Distribute())

// In relaySunoRouter (line ~195):
relaySunoRouter.Use(middleware.TokenAuth(), middleware.StrategyMiddleware(), middleware.Distribute())  // ADD StrategyMiddleware
```

Note: Do NOT add it to playgroundRouter (line ~65) as that uses `UserAuth()` not `TokenAuth()`.

- [ ] **Step 3: Verify compilation**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add middleware/strategy.go router/relay-router.go
git commit -m "feat(strategy): add StrategyMiddleware with difficulty and time strategy evaluation"
```

---

## Task 6: Strategy API Endpoints

**Files:**
- Create: `controller/strategy.go`
- Modify: `router/api-router.go`

- [ ] **Step 1: Create strategy controller**

```go
// controller/strategy.go
package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func GetAllStrategies(c *gin.Context) {
	strategies, err := model.GetAllStrategies()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    strategies,
	})
}

func GetStrategy(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	strategy, err := model.GetStrategyById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    strategy,
	})
}

func CreateStrategy(c *gin.Context) {
	var strategy model.Strategy
	if err := c.ShouldBindJSON(&strategy); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := strategy.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RefreshStrategyCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    strategy,
	})
}

func UpdateStrategy(c *gin.Context) {
	var strategy model.Strategy
	if err := c.ShouldBindJSON(&strategy); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := strategy.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RefreshStrategyCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    strategy,
	})
}

func DeleteStrategy(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	strategy := model.Strategy{Id: id}
	if err := strategy.Delete(); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RefreshStrategyCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func TestClassifier(c *gin.Context) {
	var req struct {
		ClassifierType      string `json:"classifier_type"`
		ClassifierChannelId int    `json:"classifier_channel_id"`
		ClassifierModel     string `json:"classifier_model"`
		ClassifierApiKey    string `json:"classifier_api_key"`
		ClassifierBaseUrl   string `json:"classifier_base_url"`
		ClassifierPrompt    string `json:"classifier_prompt"`
		ClassifierTimeout   int    `json:"classifier_timeout"`
		TestMessage         string `json:"test_message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	if req.ClassifierTimeout == 0 {
		req.ClassifierTimeout = 3000
	}

	messages := []map[string]string{
		{"role": "user", "content": req.TestMessage},
	}

	result, err := service.ClassifyDifficulty(
		0, // No strategy ID for test
		req.ClassifierType,
		req.ClassifierChannelId,
		req.ClassifierModel,
		req.ClassifierApiKey,
		req.ClassifierBaseUrl,
		req.ClassifierPrompt,
		req.ClassifierTimeout,
		messages,
	)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

func GetStrategyLogs(c *gin.Context) {
	strategyId, _ := strconv.Atoi(c.Query("strategy_id"))
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	logs, total, err := model.GetStrategyLogs(strategyId, page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
		"total":   total,
	})
}
```

- [ ] **Step 2: Register strategy API routes**

In `router/api-router.go`, add a new route group after the channel routes:

```go
strategyRoute := apiRouter.Group("/strategy")
strategyRoute.Use(middleware.AdminAuth())
{
    strategyRoute.GET("/", controller.GetAllStrategies)
    strategyRoute.GET("/:id", controller.GetStrategy)
    strategyRoute.POST("/", controller.CreateStrategy)
    strategyRoute.PUT("/", controller.UpdateStrategy)
    strategyRoute.DELETE("/:id", controller.DeleteStrategy)
    strategyRoute.POST("/test", controller.TestClassifier)
    strategyRoute.GET("/logs", controller.GetStrategyLogs)
}
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add controller/strategy.go router/api-router.go
git commit -m "feat(strategy): add strategy CRUD and test API endpoints"
```

---

## Task 7: Frontend Types and API

**Files:**
- Create: `web/default/src/features/system-settings/models/strategy/types.ts`
- Create: `web/default/src/features/system-settings/models/strategy/api.ts`
- Create: `web/default/src/features/system-settings/models/strategy/constants.ts`

- [ ] **Step 1: Create TypeScript types**

```typescript
// web/default/src/features/system-settings/models/strategy/types.ts
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
```

- [ ] **Step 2: Create API functions**

```typescript
// web/default/src/features/system-settings/models/strategy/api.ts
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
```

- [ ] **Step 3: Create constants**

```typescript
// web/default/src/features/system-settings/models/strategy/constants.ts
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
```

- [ ] **Step 4: Commit**

```bash
git add web/default/src/features/system-settings/models/strategy/
git commit -m "feat(strategy): add frontend types, API, and constants"
```

---

## Task 8: Frontend Strategy Management Page

**Files:**
- Create: `web/default/src/features/system-settings/models/strategy/index.tsx`
- Modify: `web/default/src/features/system-settings/models/section-registry.tsx`

- [ ] **Step 1: Create strategy section component**

```tsx
// web/default/src/features/system-settings/models/strategy/index.tsx
import { useCallback, useEffect, useState } from 'react'
import { Edit, Plus, Trash2, Zap, Clock, Play } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { SettingsSection } from '../../components/settings-section'
import { useUpdateOption } from '../../hooks/use-update-option'
import { getStrategies, deleteStrategy, updateStrategy } from './api'
import type { Strategy } from './types'
import { DifficultyStrategyDialog } from './difficulty-strategy-dialog'
import { TimeStrategyDialog } from './time-strategy-dialog'
import { StrategyTestDialog } from './strategy-test-dialog'

export function StrategySection() {
  const { t } = useTranslation()
  const [strategies, setStrategies] = useState<Strategy[]>([])
  const [loading, setLoading] = useState(true)
  const [editDialog, setEditDialog] = useState<{
    open: boolean
    type: 'difficulty' | 'time'
    strategy?: Strategy
  }>({ open: false, type: 'difficulty' })
  const [testDialog, setTestDialog] = useState<{
    open: boolean
    strategy?: Strategy
  }>({ open: false })

  const loadStrategies = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getStrategies()
      if (res.success) {
        setStrategies(res.data)
      }
    } catch {
      toast.error('Failed to load strategies')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadStrategies()
  }, [loadStrategies])

  const handleDelete = async (id: number) => {
    try {
      const res = await deleteStrategy(id)
      if (res.success) {
        toast.success('Strategy deleted')
        loadStrategies()
      }
    } catch {
      toast.error('Failed to delete strategy')
    }
  }

  const handleToggle = async (strategy: Strategy) => {
    try {
      await updateStrategy({ ...strategy, enabled: !strategy.enabled })
      loadStrategies()
    } catch {
      toast.error('Failed to update strategy')
    }
  }

  const difficultyStrategies = strategies.filter((s) => s.type === 'difficulty')
  const timeStrategies = strategies.filter((s) => s.type === 'time')

  return (
    <SettingsSection
      title={t('Routing Strategy')}
      description={t('Configure dynamic routing strategies based on query difficulty or time schedule')}
    >
      <div className='flex items-center gap-2'>
        <Button
          size='sm'
          onClick={() => setEditDialog({ open: true, type: 'difficulty' })}
        >
          <Plus className='mr-1 h-4 w-4' />
          {t('Add Difficulty Strategy')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          onClick={() => setEditDialog({ open: true, type: 'time' })}
        >
          <Plus className='mr-1 h-4 w-4' />
          {t('Add Time Strategy')}
        </Button>
      </div>

      {/* Difficulty Strategies */}
      {difficultyStrategies.length > 0 && (
        <div className='mt-4 space-y-3'>
          <h3 className='text-sm font-medium text-muted-foreground'>
            {t('Difficulty Strategies')}
          </h3>
          {difficultyStrategies.map((s) => (
            <StrategyCard
              key={s.id}
              strategy={s}
              onEdit={() =>
                setEditDialog({ open: true, type: 'difficulty', strategy: s })
              }
              onDelete={() => handleDelete(s.id)}
              onToggle={() => handleToggle(s)}
              onTest={() => setTestDialog({ open: true, strategy: s })}
            />
          ))}
        </div>
      )}

      {/* Time Strategies */}
      {timeStrategies.length > 0 && (
        <div className='mt-4 space-y-3'>
          <h3 className='text-sm font-medium text-muted-foreground'>
            {t('Time Strategies')}
          </h3>
          {timeStrategies.map((s) => (
            <StrategyCard
              key={s.id}
              strategy={s}
              onEdit={() =>
                setEditDialog({ open: true, type: 'time', strategy: s })
              }
              onDelete={() => handleDelete(s.id)}
              onToggle={() => handleToggle(s)}
            />
          ))}
        </div>
      )}

      {strategies.length === 0 && !loading && (
        <p className='text-sm text-muted-foreground'>
          {t('No strategies configured')}
        </p>
      )}

      {/* Edit Dialogs */}
      {editDialog.open && editDialog.type === 'difficulty' && (
        <DifficultyStrategyDialog
          open={editDialog.open}
          onOpenChange={(open) => setEditDialog({ ...editDialog, open })}
          strategy={editDialog.strategy}
          onSaved={loadStrategies}
        />
      )}
      {editDialog.open && editDialog.type === 'time' && (
        <TimeStrategyDialog
          open={editDialog.open}
          onOpenChange={(open) => setEditDialog({ ...editDialog, open })}
          strategy={editDialog.strategy}
          onSaved={loadStrategies}
        />
      )}
      {testDialog.open && (
        <StrategyTestDialog
          open={testDialog.open}
          onOpenChange={(open) => setTestDialog({ ...testDialog, open })}
          strategy={testDialog.strategy}
        />
      )}
    </SettingsSection>
  )
}

function StrategyCard(props: {
  strategy: Strategy
  onEdit: () => void
  onDelete: () => void
  onToggle: () => void
  onTest?: () => void
}) {
  const { t } = useTranslation()
  const { strategy, onEdit, onDelete, onToggle, onTest } = props

  return (
    <Card>
      <CardHeader className='flex flex-row items-center justify-between space-y-0 pb-2'>
        <div className='flex items-center gap-2'>
          {strategy.type === 'difficulty' ? (
            <Zap className='h-4 w-4 text-yellow-500' />
          ) : (
            <Clock className='h-4 w-4 text-blue-500' />
          )}
          <CardTitle className='text-base'>{strategy.name}</CardTitle>
          <Badge variant={strategy.enabled ? 'default' : 'secondary'}>
            {strategy.enabled ? t('Enabled') : t('Disabled')}
          </Badge>
        </div>
        <div className='flex items-center gap-2'>
          <Switch
            checked={strategy.enabled}
            onCheckedChange={onToggle}
          />
          {onTest && (
            <Button size='sm' variant='ghost' onClick={onTest}>
              <Play className='h-4 w-4' />
            </Button>
          )}
          <Button size='sm' variant='ghost' onClick={onEdit}>
            <Edit className='h-4 w-4' />
          </Button>
          <Button size='sm' variant='ghost' onClick={onDelete}>
            <Trash2 className='h-4 w-4' />
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className='text-sm text-muted-foreground'>
          {t('Priority')}: {strategy.priority}
          {strategy.type === 'difficulty' && (
            <> · {t('Model')}: {strategy.classifier_model}</>
          )}
          {strategy.type === 'time' && (
            <> · {t('Cron')}: {strategy.cron_expr}</>
          )}
        </div>
        {strategy.description && (
          <p className='mt-1 text-sm text-muted-foreground'>
            {strategy.description}
          </p>
        )}
      </CardContent>
    </Card>
  )
}
```

- [ ] **Step 2: Register strategy section in section-registry.tsx**

Add to the `MODELS_SECTIONS` array in `web/default/src/features/system-settings/models/section-registry.tsx`:

```tsx
import { StrategySection } from './strategy'

// In MODELS_SECTIONS array, add after channel-affinity:
{
  id: 'strategy',
  titleKey: 'Routing Strategy',
  build: () => <StrategySection />,
},
```

Also update the `ModelSectionId` type to include `'strategy'`.

- [ ] **Step 3: Add navigation item**

In `web/default/src/components/layout/config/system-settings.config.ts`, add the strategy item to the Models section's `items` array (following the channel-affinity item pattern).

- [ ] **Step 4: Verify frontend builds**

```bash
cd web/default
bun run build
```

- [ ] **Step 5: Commit**

```bash
git add web/default/src/features/system-settings/models/strategy/index.tsx
git add web/default/src/features/system-settings/models/section-registry.tsx
git add web/default/src/components/layout/config/system-settings.config.ts
git commit -m "feat(strategy): add strategy management page and register in settings"
```

---

## Task 9: Difficulty Strategy Edit Dialog

**Files:**
- Create: `web/default/src/features/system-settings/models/strategy/difficulty-strategy-dialog.tsx`

- [ ] **Step 1: Create difficulty strategy dialog**

```tsx
// web/default/src/features/system-settings/models/strategy/difficulty-strategy-dialog.tsx
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { MultiSelect } from '@/components/ui/multi-select'
import { createStrategy, updateStrategy } from './api'
import type { Strategy } from './types'
import { z } from 'zod'

const formSchema = z.object({
  name: z.string().min(1),
  priority: z.number().default(0),
  enabled: z.boolean().default(true),
  classifier_type: z.enum(['channel', 'independent']),
  classifier_channel_id: z.number().optional(),
  classifier_model: z.string().min(1),
  classifier_api_key: z.string().optional(),
  classifier_base_url: z.string().optional(),
  classifier_prompt: z.string().optional(),
  classifier_timeout: z.number().default(3000),
  difficulty_models: z.object({
    simple: z.array(z.string()),
    medium: z.array(z.string()),
    hard: z.array(z.string()),
  }),
  description: z.string().optional(),
})

type FormData = z.infer<typeof formSchema>

export function DifficultyStrategyDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  strategy?: Strategy
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const { open, onOpenChange, strategy, onSaved } = props
  const [saving, setSaving] = useState(false)
  const isEdit = !!strategy

  const form = useForm<FormData>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      name: strategy?.name ?? '',
      priority: strategy?.priority ?? 0,
      enabled: strategy?.enabled ?? true,
      classifier_type: (strategy?.classifier_type as 'channel' | 'independent') ?? 'channel',
      classifier_channel_id: strategy?.classifier_channel_id ?? 0,
      classifier_model: strategy?.classifier_model ?? '',
      classifier_api_key: strategy?.classifier_api_key ?? '',
      classifier_base_url: strategy?.classifier_base_url ?? '',
      classifier_prompt: strategy?.classifier_prompt ?? '',
      classifier_timeout: strategy?.classifier_timeout ?? 3000,
      difficulty_models: strategy?.difficulty_models
        ? JSON.parse(strategy.difficulty_models)
        : { simple: [], medium: [], hard: [] },
      description: strategy?.description ?? '',
    },
  })

  const onSubmit = async (data: FormData) => {
    setSaving(true)
    try {
      const payload = {
        ...strategy,
        ...data,
        type: 'difficulty' as const,
        difficulty_models: JSON.stringify(data.difficulty_models),
      }
      const res = isEdit ? await updateStrategy(payload) : await createStrategy(payload)
      if (res.success) {
        toast.success(isEdit ? 'Strategy updated' : 'Strategy created')
        onSaved()
        onOpenChange(false)
      }
    } catch {
      toast.error('Failed to save strategy')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={isEdit ? t('Edit Difficulty Strategy') : t('Create Difficulty Strategy')}
      contentClassName='sm:max-w-2xl'
    >
      <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-4'>
        {/* Name + Priority + Enabled */}
        <div className='grid grid-cols-3 gap-4'>
          <div className='col-span-2'>
            <Label>{t('Name')}</Label>
            <Input {...form.register('name')} />
          </div>
          <div>
            <Label>{t('Priority')}</Label>
            <Input type='number' {...form.register('priority', { valueAsNumber: true })} />
          </div>
        </div>

        {/* Classifier Type */}
        <div>
          <Label>{t('Classifier Type')}</Label>
          <Select
            value={form.watch('classifier_type')}
            onValueChange={(v) => form.setValue('classifier_type', v as any)}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='channel'>{t('Reuse Channel')}</SelectItem>
              <SelectItem value='independent'>{t('Independent Config')}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {/* Channel or Independent config */}
        {form.watch('classifier_type') === 'channel' ? (
          <div>
            <Label>{t('Channel ID')}</Label>
            <Input type='number' {...form.register('classifier_channel_id', { valueAsNumber: true })} />
          </div>
        ) : (
          <div className='grid grid-cols-2 gap-4'>
            <div>
              <Label>{t('API Key')}</Label>
              <Input type='password' {...form.register('classifier_api_key')} />
            </div>
            <div>
              <Label>{t('Base URL')}</Label>
              <Input {...form.register('classifier_base_url')} />
            </div>
          </div>
        )}

        {/* Model + Timeout */}
        <div className='grid grid-cols-2 gap-4'>
          <div>
            <Label>{t('Model')}</Label>
            <Input {...form.register('classifier_model')} placeholder='gpt-4o-mini' />
          </div>
          <div>
            <Label>{t('Timeout (ms)')}</Label>
            <Input type='number' {...form.register('classifier_timeout', { valueAsNumber: true })} />
          </div>
        </div>

        {/* Custom Prompt */}
        <div>
          <Label>{t('Custom Prompt (optional)')}</Label>
          <Textarea {...form.register('classifier_prompt')} rows={3} />
        </div>

        {/* Difficulty -> Model Mapping */}
        <div className='space-y-3'>
          <Label>{t('Difficulty -> Model Mapping')}</Label>
          {(['simple', 'medium', 'hard'] as const).map((level) => (
            <div key={level}>
              <Label className='text-sm capitalize'>{t(level)}</Label>
              <MultiSelect
                value={form.watch(`difficulty_models.${level}`)}
                onChange={(v) => form.setValue(`difficulty_models.${level}`, v)}
                placeholder={t('Select models...')}
              />
            </div>
          ))}
        </div>

        {/* Description */}
        <div>
          <Label>{t('Description')}</Label>
          <Input {...form.register('description')} />
        </div>

        <div className='flex justify-end gap-2'>
          <Button type='button' variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button type='submit' disabled={saving}>
            {saving ? t('Saving...') : t('Save')}
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add web/default/src/features/system-settings/models/strategy/difficulty-strategy-dialog.tsx
git commit -m "feat(strategy): add difficulty strategy edit dialog"
```

---

## Task 10: Time Strategy Edit Dialog

**Files:**
- Create: `web/default/src/features/system-settings/models/strategy/time-strategy-dialog.tsx`

- [ ] **Step 1: Create time strategy dialog**

The time strategy dialog should follow the same pattern as the difficulty strategy dialog but with:
- Cron expression input with human-readable preview
- Timezone selector
- Action configuration: enable/disable models, priority/weight adjustments

```tsx
// web/default/src/features/system-settings/models/strategy/time-strategy-dialog.tsx
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { createStrategy, updateStrategy } from './api'
import type { Strategy } from './types'
import { COMMON_TIMEZONES, DEFAULT_TIME_ACTIONS } from './constants'
import { z } from 'zod'

const formSchema = z.object({
  name: z.string().min(1),
  priority: z.number().default(0),
  enabled: z.boolean().default(true),
  cron_expr: z.string().min(1),
  timezone: z.string().default('UTC'),
  time_actions: z.object({
    enable_models: z.array(z.string()).optional(),
    disable_models: z.array(z.string()).optional(),
    use_models: z.array(z.string()).optional(),
  }),
  description: z.string().optional(),
})

type FormData = z.infer<typeof formSchema>

export function TimeStrategyDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  strategy?: Strategy
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const { open, onOpenChange, strategy, onSaved } = props
  const [saving, setSaving] = useState(false)
  const isEdit = !!strategy

  const form = useForm<FormData>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      name: strategy?.name ?? '',
      priority: strategy?.priority ?? 0,
      enabled: strategy?.enabled ?? true,
      cron_expr: strategy?.cron_expr ?? '0 9-17 * * 1-5',
      timezone: strategy?.timezone ?? 'Asia/Shanghai',
      time_actions: strategy?.time_actions
        ? JSON.parse(strategy.time_actions)
        : DEFAULT_TIME_ACTIONS,
      description: strategy?.description ?? '',
    },
  })

  const onSubmit = async (data: FormData) => {
    setSaving(true)
    try {
      const payload = {
        ...strategy,
        ...data,
        type: 'time' as const,
        time_actions: JSON.stringify(data.time_actions),
      }
      const res = isEdit ? await updateStrategy(payload) : await createStrategy(payload)
      if (res.success) {
        toast.success(isEdit ? 'Strategy updated' : 'Strategy created')
        onSaved()
        onOpenChange(false)
      }
    } catch {
      toast.error('Failed to save strategy')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={isEdit ? t('Edit Time Strategy') : t('Create Time Strategy')}
      contentClassName='sm:max-w-2xl'
    >
      <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-4'>
        <div className='grid grid-cols-3 gap-4'>
          <div className='col-span-2'>
            <Label>{t('Name')}</Label>
            <Input {...form.register('name')} />
          </div>
          <div>
            <Label>{t('Priority')}</Label>
            <Input type='number' {...form.register('priority', { valueAsNumber: true })} />
          </div>
        </div>

        <div className='grid grid-cols-2 gap-4'>
          <div>
            <Label>{t('Cron Expression')}</Label>
            <Input {...form.register('cron_expr')} placeholder='0 9-17 * * 1-5' />
            <p className='text-xs text-muted-foreground mt-1'>
              {t('Format: minute hour day-of-month month day-of-week')}
            </p>
          </div>
          <div>
            <Label>{t('Timezone')}</Label>
            <Select
              value={form.watch('timezone')}
              onValueChange={(v) => form.setValue('timezone', v)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {COMMON_TIMEZONES.map((tz) => (
                  <SelectItem key={tz} value={tz}>
                    {tz}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        {/* Actions */}
        <div className='space-y-3'>
          <Label>{t('Actions')}</Label>
          <div>
            <Label className='text-sm'>{t('Use Models (replace request model)')}</Label>
            <Input
              value={form.watch('time_actions.use_models')?.join(', ') ?? ''}
              onChange={(e) =>
                form.setValue(
                  'time_actions.use_models',
                  e.target.value.split(',').map((s) => s.trim()).filter(Boolean)
                )
              }
              placeholder='gpt-4o-mini, gemini-2.0-flash'
            />
          </div>
          <div>
            <Label className='text-sm'>{t('Disable Models')}</Label>
            <Input
              value={form.watch('time_actions.disable_models')?.join(', ') ?? ''}
              onChange={(e) =>
                form.setValue(
                  'time_actions.disable_models',
                  e.target.value.split(',').map((s) => s.trim()).filter(Boolean)
                )
              }
              placeholder='gpt-4o, claude-3.5-sonnet'
            />
          </div>
        </div>

        <div>
          <Label>{t('Description')}</Label>
          <Input {...form.register('description')} />
        </div>

        <div className='flex justify-end gap-2'>
          <Button type='button' variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button type='submit' disabled={saving}>
            {saving ? t('Saving...') : t('Save')}
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add web/default/src/features/system-settings/models/strategy/time-strategy-dialog.tsx
git commit -m "feat(strategy): add time strategy edit dialog"
```

---

## Task 11: Strategy Test Dialog and Logs View

**Files:**
- Create: `web/default/src/features/system-settings/models/strategy/strategy-test-dialog.tsx`
- Create: `web/default/src/features/system-settings/models/strategy/strategy-logs-table.tsx`

- [ ] **Step 1: Create test dialog**

```tsx
// web/default/src/features/system-settings/models/strategy/strategy-test-dialog.tsx
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { testClassifier } from './api'
import type { Strategy, ClassifierResult } from './types'

export function StrategyTestDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  strategy?: Strategy
}) {
  const { t } = useTranslation()
  const { open, onOpenChange, strategy } = props
  const [testMessage, setTestMessage] = useState('')
  const [testing, setTesting] = useState(false)
  const [result, setResult] = useState<ClassifierResult | null>(null)
  const [error, setError] = useState('')

  const handleTest = async () => {
    if (!strategy || !testMessage.trim()) return
    setTesting(true)
    setError('')
    setResult(null)

    try {
      const res = await testClassifier({
        classifier_type: strategy.classifier_type,
        classifier_channel_id: strategy.classifier_channel_id,
        classifier_model: strategy.classifier_model,
        classifier_api_key: strategy.classifier_api_key,
        classifier_base_url: strategy.classifier_base_url,
        classifier_prompt: strategy.classifier_prompt,
        classifier_timeout: strategy.classifier_timeout,
        test_message: testMessage,
      })

      if (res.success && res.data) {
        setResult(res.data)
      } else {
        setError(res.message ?? 'Classification failed')
      }
    } catch (e: any) {
      setError(e.message ?? 'Request failed')
    } finally {
      setTesting(false)
    }
  }

  const levelColors: Record<string, string> = {
    simple: 'bg-green-100 text-green-800',
    medium: 'bg-yellow-100 text-yellow-800',
    hard: 'bg-red-100 text-red-800',
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Test Classifier')}
      contentClassName='sm:max-w-lg'
    >
      <div className='space-y-4'>
        <div>
          <Label>{t('Test Message')}</Label>
          <Textarea
            value={testMessage}
            onChange={(e) => setTestMessage(e.target.value)}
            placeholder={t('Enter a test message to classify...')}
            rows={3}
          />
        </div>

        <Button onClick={handleTest} disabled={testing || !testMessage.trim()}>
          {testing ? t('Testing...') : t('Run Test')}
        </Button>

        {result && (
          <div className='rounded-lg border p-4 space-y-2'>
            <div className='flex items-center gap-2'>
              <span className='text-sm font-medium'>{t('Level')}:</span>
              <Badge className={levelColors[result.level] ?? ''}>
                {result.level}
              </Badge>
            </div>
            <div className='text-sm text-muted-foreground'>
              {result.reason}
            </div>
          </div>
        )}

        {error && (
          <div className='rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-800'>
            {error}
          </div>
        )}
      </div>
    </Dialog>
  )
}
```

- [ ] **Step 2: Create logs table component**

```tsx
// web/default/src/features/system-settings/models/strategy/strategy-logs-table.tsx
import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { getStrategyLogs } from './api'
import type { StrategyLog } from './types'

export function StrategyLogsTable(props: { strategyId?: number }) {
  const { t } = useTranslation()
  const [logs, setLogs] = useState<StrategyLog[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const pageSize = 20

  const loadLogs = useCallback(async () => {
    const res = await getStrategyLogs({
      strategy_id: props.strategyId,
      p: page,
      size: pageSize,
    })
    if (res.success) {
      setLogs(res.data)
      setTotal(res.total)
    }
  }, [page, props.strategyId])

  useEffect(() => {
    loadLogs()
  }, [loadLogs])

  const resultColors: Record<string, string> = {
    simple: 'bg-green-100 text-green-800',
    medium: 'bg-yellow-100 text-yellow-800',
    hard: 'bg-red-100 text-red-800',
    fallback: 'bg-gray-100 text-gray-800',
    time_match: 'bg-blue-100 text-blue-800',
  }

  return (
    <div className='space-y-2'>
      <h3 className='text-sm font-medium text-muted-foreground'>
        {t('Strategy Logs')}
      </h3>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Time')}</TableHead>
            <TableHead>{t('Result')}</TableHead>
            <TableHead>{t('Latency')}</TableHead>
            <TableHead>{t('Error')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {logs.map((log) => (
            <TableRow key={log.id}>
              <TableCell className='text-sm'>
                {new Date(log.created_at * 1000).toLocaleString()}
              </TableCell>
              <TableCell>
                <Badge className={resultColors[log.result] ?? ''}>
                  {log.result}
                </Badge>
              </TableCell>
              <TableCell className='text-sm'>{log.latency_ms}ms</TableCell>
              <TableCell className='text-sm text-red-600 max-w-xs truncate'>
                {log.error}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <div className='flex items-center justify-between'>
        <span className='text-sm text-muted-foreground'>
          {t('Total')}: {total}
        </span>
        <div className='flex gap-2'>
          <Button
            size='sm'
            variant='outline'
            disabled={page <= 1}
            onClick={() => setPage(page - 1)}
          >
            {t('Previous')}
          </Button>
          <Button
            size='sm'
            variant='outline'
            disabled={page * pageSize >= total}
            onClick={() => setPage(page + 1)}
          >
            {t('Next')}
          </Button>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 3: Commit**

```bash
git add web/default/src/features/system-settings/models/strategy/strategy-test-dialog.tsx
git add web/default/src/features/system-settings/models/strategy/strategy-logs-table.tsx
git commit -m "feat(strategy): add test dialog and logs table"
```

---

## Task 12: Integration Testing

- [ ] **Step 1: Run backend tests**

```bash
go test ./...
```

- [ ] **Step 2: Run frontend build**

```bash
cd web/default
bun run build
```

- [ ] **Step 3: Manual testing checklist**

1. Start the application
2. Navigate to System Settings -> Models & Routing -> Routing Strategy
3. Create a difficulty strategy:
   - Set classifier to "Reuse Channel", select a channel, set model to "gpt-4o-mini"
   - Configure difficulty -> model mapping for all three levels
   - Save and verify it appears in the list
4. Test the classifier:
   - Click the test button, enter "Hello, how are you?"
   - Verify it returns "simple"
   - Enter "Write a binary search tree implementation in Go"
   - Verify it returns "medium" or "hard"
5. Create a time strategy:
   - Set cron to current time window
   - Configure use_models action
   - Save and verify
6. Send a relay request and verify:
   - Strategy middleware classifies the request
   - Distribute selects the correct model based on classification
7. Check strategy logs in the UI

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat(strategy): complete strategy routing system"
```
