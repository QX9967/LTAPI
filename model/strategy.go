package model

import (
	"time"

	"gorm.io/gorm"
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

func (s *Strategy) BeforeCreate(tx *gorm.DB) error {
	if s.CreatedAt == 0 {
		s.CreatedAt = time.Now().Unix()
	}
	if s.UpdatedAt == 0 {
		s.UpdatedAt = time.Now().Unix()
	}
	return nil
}

func (s *Strategy) BeforeUpdate(tx *gorm.DB) error {
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

func (l *StrategyLog) BeforeCreate(tx *gorm.DB) error {
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
