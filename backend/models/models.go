package models

import (
	"encoding/json"
	"time"
)

type PlankroadSite struct {
	SiteID          int       `json:"site_id" db:"site_id"`
	SiteName        string    `json:"site_name" db:"site_name"`
	Region          string    `json:"region" db:"region"`
	Elevation       float64   `json:"elevation" db:"elevation"`
	ConstructionEra string    `json:"construction_era" db:"construction_era"`
	TotalLength     float64   `json:"total_length" db:"total_length"`
	BeamCount       int       `json:"beam_count" db:"beam_count"`
	RockType        string    `json:"rock_type" db:"rock_type"`
	WoodType        string    `json:"wood_type" db:"wood_type"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

type SensorReading struct {
	Time              time.Time `json:"time" db:"time"`
	SiteID            int       `json:"site_id" db:"site_id"`
	BeamID            int       `json:"beam_id" db:"beam_id"`
	BeamStrainTop     float64   `json:"beam_strain_top" db:"beam_strain_top"`
	BeamStrainBottom  float64   `json:"beam_strain_bottom" db:"beam_strain_bottom"`
	BeamStrainSide    float64   `json:"beam_strain_side" db:"beam_strain_side"`
	RockCrackWidth1   float64   `json:"rock_crack_width_1" db:"rock_crack_width_1"`
	RockCrackWidth2   float64   `json:"rock_crack_width_2" db:"rock_crack_width_2"`
	RockCrackWidth3   float64   `json:"rock_crack_width_3" db:"rock_crack_width_3"`
	Temperature       float64   `json:"temperature" db:"temperature"`
	Humidity          float64   `json:"humidity" db:"humidity"`
	Rainfall          float64   `json:"rainfall" db:"rainfall"`
	AvgStrain         float64   `json:"avg_strain,omitempty" db:"avg_strain"`
	MaxCrackWidth     float64   `json:"max_crack_width,omitempty" db:"max_crack_width"`
	StrainAlarm       bool      `json:"strain_alarm,omitempty" db:"strain_alarm"`
	CrackAlarm        bool      `json:"crack_alarm,omitempty" db:"crack_alarm"`
}

type StructuralSimulation struct {
	SimID              int             `json:"sim_id" db:"sim_id"`
	SiteID             int             `json:"site_id" db:"site_id"`
	SimTime            time.Time       `json:"sim_time" db:"sim_time"`
	WoodElasticModulus float64         `json:"wood_elastic_modulus" db:"wood_elastic_modulus"`
	RockElasticModulus float64         `json:"rock_elastic_modulus" db:"rock_elastic_modulus"`
	WoodPoissonRatio   float64         `json:"wood_poisson_ratio" db:"wood_poisson_ratio"`
	RockPoissonRatio   float64         `json:"rock_poisson_ratio" db:"rock_poisson_ratio"`
	DeadLoad           float64         `json:"dead_load" db:"dead_load"`
	LiveLoad           float64         `json:"live_load" db:"live_load"`
	ThermalLoad        float64         `json:"thermal_load" db:"thermal_load"`
	MaxWoodStress      float64         `json:"max_wood_stress" db:"max_wood_stress"`
	MinWoodStress      float64         `json:"min_wood_stress" db:"min_wood_stress"`
	MaxRockStress      float64         `json:"max_rock_stress" db:"max_rock_stress"`
	MinRockStress      float64         `json:"min_rock_stress" db:"min_rock_stress"`
	MaxDeflectionMM    float64         `json:"max_deflection_mm" db:"max_deflection_mm"`
	SafetyFactor       float64         `json:"safety_factor" db:"safety_factor"`
	ElementData        json.RawMessage `json:"element_data,omitempty" db:"element_data"`
	CreatedAt          time.Time       `json:"created_at,omitempty" db:"created_at"`
}

type FEMNode struct {
	ID        int       `json:"id"`
	X, Y, Z   float64   `json:"x,yz"`
	Material  string    `json:"material"`
	StressXX  float64   `json:"stress_xx"`
	StressYY  float64   `json:"stress_yy"`
	StressZZ  float64   `json:"stress_zz"`
	StressXY  float64   `json:"stress_xy"`
	VonMises  float64   `json:"von_mises"`
	DisplacementX float64 `json:"dx"`
	DisplacementY float64 `json:"dy"`
	DisplacementZ float64 `json:"dz"`
}

type FEMElement struct {
	ID      int       `json:"id"`
	NodeIDs [4]int    `json:"node_ids"`
	Material string    `json:"material"`
	Stress  float64   `json:"stress"`
	Strain  float64   `json:"strain"`
}

type WeatheringAssessment struct {
	AssessID           int             `json:"assess_id" db:"assess_id"`
	SiteID             int             `json:"site_id" db:"site_id"`
	AssessTime         time.Time       `json:"assess_time" db:"assess_time"`
	FreezeThawCycles   int             `json:"freeze_thaw_cycles" db:"freeze_thaw_cycles"`
	CurrentCrackDepth  float64         `json:"current_crack_depth" db:"current_crack_depth"`
	CrackPropagationRate float64       `json:"crack_propagation_rate" db:"crack_propagation_rate"`
	WeatheringGrade    string          `json:"weathering_grade" db:"weathering_grade"`
	WoodDecayRate      float64         `json:"wood_decay_rate" db:"wood_decay_rate"`
	RockErosionRate    float64         `json:"rock_erosion_rate" db:"rock_erosion_rate"`
	PredictedLifespan  float64         `json:"predicted_lifespan" db:"predicted_lifespan"`
	RemainingLifespan  float64         `json:"remaining_lifespan" db:"remaining_lifespan"`
	Confidence         float64         `json:"confidence" db:"confidence"`
	DetailData         json.RawMessage `json:"detail_data,omitempty" db:"detail_data"`
	CreatedAt          time.Time       `json:"created_at,omitempty" db:"created_at"`
}

type AlarmEvent struct {
	AlarmID        int64     `json:"alarm_id" db:"alarm_id"`
	Time           time.Time `json:"time" db:"time"`
	SiteID         int       `json:"site_id" db:"site_id"`
	BeamID         int       `json:"beam_id,omitempty" db:"beam_id"`
	AlarmType      string    `json:"alarm_type" db:"alarm_type"`
	AlarmLevel     string    `json:"alarm_level" db:"alarm_level"`
	MetricName     string    `json:"metric_name,omitempty" db:"metric_name"`
	CurrentValue   float64   `json:"current_value,omitempty" db:"current_value"`
	ThresholdValue float64   `json:"threshold_value,omitempty" db:"threshold_value"`
	Description    string    `json:"description" db:"description"`
	Acknowledged   bool      `json:"acknowledged,omitempty" db:"acknowledged"`
	Resolved       bool      `json:"resolved,omitempty" db:"resolved"`
	MQTTPublished  bool      `json:"mqtt_published,omitempty" db:"mqtt_published"`
	CreatedAt      time.Time `json:"created_at,omitempty" db:"created_at"`
}

type AlarmThreshold struct {
	ThresholdID       int     `json:"threshold_id" db:"threshold_id"`
	SiteID            int     `json:"site_id" db:"site_id"`
	StrainWarning     float64 `json:"strain_warning" db:"strain_warning"`
	StrainCritical    float64 `json:"strain_critical" db:"strain_critical"`
	CrackWarning      float64 `json:"crack_warning" db:"crack_warning"`
	CrackCritical     float64 `json:"crack_critical" db:"crack_critical"`
	CrackRateWarning  float64 `json:"crack_rate_warning" db:"crack_rate_warning"`
	CrackRateCritical float64 `json:"crack_rate_critical" db:"crack_rate_critical"`
	SFWarning         float64 `json:"sf_warning" db:"sf_warning"`
	SFCritical        float64 `json:"sf_critical" db:"sf_critical"`
}

type DailySummary struct {
	SiteID         int       `json:"site_id" db:"site_id"`
	BucketDay      time.Time `json:"bucket_day" db:"bucket_day"`
	ReadingCount   int64     `json:"reading_count" db:"reading_count"`
	AvgDailyStrain float64   `json:"avg_daily_strain" db:"avg_daily_strain"`
	MaxDailyStrain float64   `json:"max_daily_strain" db:"max_daily_strain"`
	AvgDailyCrack  float64   `json:"avg_daily_crack" db:"avg_daily_crack"`
	MaxDailyCrack  float64   `json:"max_daily_crack" db:"max_daily_crack"`
	AvgTemp        float64   `json:"avg_temp" db:"avg_temp"`
	MinTemp        float64   `json:"min_temp" db:"min_temp"`
	MaxTemp        float64   `json:"max_temp" db:"max_temp"`
	AvgHumidity    float64   `json:"avg_humidity" db:"avg_humidity"`
	AlarmCount     int64     `json:"alarm_count" db:"alarm_count"`
}
