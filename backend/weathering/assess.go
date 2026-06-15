package weathering

import (
	"encoding/json"
	"math"
	"time"

	"plankroad-backend/config"
	"plankroad-backend/models"
)

type FreezeThawState int

const (
	StateNormal FreezeThawState = iota
	StateFreezing
	StateFrozen
	StateThawing
)

type FreezeThawTracker struct {
	CurrentState     FreezeThawState
	Cycles           int
	LastStateChange  time.Time
	FreezeDuration   time.Duration
	ThawDuration     time.Duration
}

type RockDecayParams struct {
	K_IC             float64
	C                float64
	m                float64
	PoreWaterFrac    float64
	FreezePressure   float64
}

type WoodDecayParams struct {
	InitMoisture     float64
	DecayCoeff       float64
	TempCoeff        float64
	HumidCoeff       float64
	BiologicalFactor float64
}

type Assessor struct {
	cfg       *config.WeatherConfig
	ftTrackers map[int]*FreezeThawTracker
}

func NewAssessor(cfg *config.WeatherConfig) *Assessor {
	return &Assessor{
		cfg:       cfg,
		ftTrackers: make(map[int]*FreezeThawTracker),
	}
}

var RockParams = map[string]RockDecayParams{
	"石灰岩":  {K_IC: 1.5, C: 1.0e-12, m: 3.0, PoreWaterFrac: 0.03, FreezePressure: 2.5},
	"花岗岩":  {K_IC: 2.5, C: 5.0e-13, m: 3.2, PoreWaterFrac: 0.01, FreezePressure: 1.8},
	"片麻岩":  {K_IC: 1.8, C: 8.0e-13, m: 3.1, PoreWaterFrac: 0.015, FreezePressure: 2.0},
	"大理岩":  {K_IC: 1.3, C: 1.2e-12, m: 2.9, PoreWaterFrac: 0.025, FreezePressure: 2.3},
	"砂岩":   {K_IC: 0.8, C: 2.0e-12, m: 2.8, PoreWaterFrac: 0.08, FreezePressure: 3.5},
	"板岩":   {K_IC: 1.0, C: 1.5e-12, m: 2.85, PoreWaterFrac: 0.05, FreezePressure: 3.0},
}

var WoodParams = map[string]WoodDecayParams{
	"柏木":   {InitMoisture: 12.0, DecayCoeff: 0.004, TempCoeff: 0.08, HumidCoeff: 0.005, BiologicalFactor: 0.7},
	"青冈木":  {InitMoisture: 10.0, DecayCoeff: 0.003, TempCoeff: 0.07, HumidCoeff: 0.004, BiologicalFactor: 0.6},
	"松木":   {InitMoisture: 15.0, DecayCoeff: 0.006, TempCoeff: 0.10, HumidCoeff: 0.006, BiologicalFactor: 0.9},
	"栎木":   {InitMoisture: 11.0, DecayCoeff: 0.0035, TempCoeff: 0.075, HumidCoeff: 0.0045, BiologicalFactor: 0.65},
	"杉木":   {InitMoisture: 14.0, DecayCoeff: 0.005, TempCoeff: 0.09, HumidCoeff: 0.0055, BiologicalFactor: 0.8},
}

func (a *Assessor) Assess(site *models.PlankroadSite, readings []models.SensorReading, sim *models.StructuralSimulation) *models.WeatheringAssessment {
	if tracker, ok := a.ftTrackers[site.SiteID]; !ok {
		a.ftTrackers[site.SiteID] = &FreezeThawTracker{}
	}

	cycles := a.calcFreezeThawCycles(site.SiteID, readings)
	avgTemp, avgHumid, tempRange := calcClimateParams(readings)

	rockParams := getRockParams(site.RockType)
	woodParams := getWoodParams(site.WoodType)

	avgCrack := 0.0
	maxCrack := 0.0
	crackRate := 0.0
	if len(readings) > 0 {
		for _, r := range readings {
			avgCrack += r.MaxCrackWidth
			if r.MaxCrackWidth > maxCrack {
				maxCrack = r.MaxCrackWidth
			}
		}
		avgCrack /= float64(len(readings))
	}

	stressRange := 0.0
	if sim != nil {
		stressRange = math.Max(sim.MaxRockStress-sim.MinRockStress, 5.0)
	}

	crackPropRate := a.calcCrackPropagation(rockParams, stressRange, avgCrack, cycles)
	crackRate = crackPropRate * 12.0

	weatherGrade := a.gradeWeathering(avgCrack, crackRate, cycles)

	woodDecay := a.calcWoodDecayRate(woodParams, avgTemp, avgHumid, cycles)
	rockErosion := a.calcRockErosionRate(rockParams, avgHumid, tempRange, cycles)

	currentAge := a.estimateCurrentAge(site)

	rockLife := a.predictRockLifespan(maxCrack, crackPropRate)
	woodLife := a.predictWoodLifespan(woodDecay, currentAge)
	predictedLife := math.Min(rockLife, woodLife)

	remaining := math.Max(predictedLife-currentAge, 0.1)
	confidence := a.calcConfidence(len(readings), cycles, stressRange)

	detail := map[string]interface{}{
		"avg_temperature":        round4(avgTemp),
		"avg_humidity":           round4(avgHumid),
		"temperature_range":      round4(tempRange),
		"avg_crack_width_mm":     round4(avgCrack),
		"max_crack_width_mm":     round4(maxCrack),
		"monthly_crack_rate_mm":  round4(crackRate),
		"rock_fracture_toughness": rockParams.K_IC,
		"rock_pore_water_frac":   rockParams.PoreWaterFrac,
		"wood_moisture_content":  round4(woodParams.InitMoisture + avgHumid*0.1),
		"estimated_age_years":    round4(currentAge),
		"rock_lifespan_years":    round4(rockLife),
		"wood_lifespan_years":    round4(woodLife),
		"freeze_pressure_mpa":    rockParams.FreezePressure,
	}
	detailJSON, _ := json.Marshal(detail)

	return &models.WeatheringAssessment{
		SiteID:              site.SiteID,
		AssessTime:          time.Now(),
		FreezeThawCycles:    cycles,
		CurrentCrackDepth:   round4(maxCrack),
		CrackPropagationRate: round4(crackPropRate),
		WeatheringGrade:     weatherGrade,
		WoodDecayRate:       round4(woodDecay),
		RockErosionRate:     round4(rockErosion),
		PredictedLifespan:   round4(predictedLife),
		RemainingLifespan:   round4(remaining),
		Confidence:          round4(confidence),
		DetailData:          detailJSON,
	}
}

func (a *Assessor) calcFreezeThawCycles(siteID int, readings []models.SensorReading) int {
	if len(readings) < 2 {
		return 80
	}

	tracker := a.ftTrackers[siteID]
	baseCycles := tracker.Cycles

	prevTemp := readings[len(readings)-1].Temperature
	transitions := 0

	for i := len(readings) - 2; i >= 0; i-- {
		currTemp := readings[i].Temperature
		if prevTemp > a.cfg.ThawTemp && currTemp < a.cfg.FreezeTemp {
			transitions++
		} else if prevTemp < a.cfg.FreezeTemp && currTemp > a.cfg.ThawTemp {
			transitions++
		}
		prevTemp = currTemp
	}

	newCycles := transitions / 2
	tracker.Cycles = baseCycles + newCycles

	return baseCycles + newCycles + 80
}

func calcClimateParams(readings []models.SensorReading) (avgTemp, avgHumid, tempRange float64) {
	if len(readings) == 0 {
		return 15.0, 70.0, 25.0
	}

	minT, maxT := 1e10, -1e10
	for _, r := range readings {
		avgTemp += r.Temperature
		avgHumid += r.Humidity
		if r.Temperature < minT {
			minT = r.Temperature
		}
		if r.Temperature > maxT {
			maxT = r.Temperature
		}
	}
	n := float64(len(readings))
	avgTemp /= n
	avgHumid /= n
	tempRange = math.Max(maxT-minT, 15.0)
	return
}

func (a *Assessor) calcCrackPropagation(params RockDecayParams, stressRange, crackWidth float64, cycles int) float64 {
	pi := math.Pi
	geoFactor := 1.12
	deltaK := geoFactor * stressRange * math.Sqrt(pi*crackWidth/1000.0)

	if crackWidth <= 0.01 {
		crackWidth = 0.1
	}

	if deltaK >= params.K_IC {
		return a.cfg.CriticalCrackDepth / 50.0
	}

	freezeDamage := 1.0 + params.PoreWaterFrac*params.FreezePressure*float64(cycles)/200.0

	da_dN := params.C * math.Pow(deltaK, params.m) * freezeDamage

	annualCycles := math.Max(float64(cycles), 80.0)
	annualGrowth := da_dN * 1e3 * annualCycles

	minRate := 0.001 + params.PoreWaterFrac*0.05
	return math.Max(annualGrowth, minRate)
}

func (a *Assessor) calcWoodDecayRate(params WoodDecayParams, avgTemp, avgHumid float64, cycles int) float64 {
	tempFactor := math.Exp(params.TempCoeff * (avgTemp - 20.0))
	humidFactor := 1.0 + params.HumidCoeff*(avgHumid-65.0)
	if humidFactor < 0.1 {
		humidFactor = 0.1
	}

	ftFactor := 1.0 + float64(cycles)*0.001
	moisture := params.InitMoisture + avgHumid*0.08

	var moistureFactor float64
	if moisture < 20 {
		moistureFactor = 0.1 + (moisture / 20.0) * 0.5
	} else if moisture <= 40 {
		moistureFactor = 0.6 + ((moisture - 20.0) / 20.0) * 0.4
	} else {
		moistureFactor = 1.0 + (moisture-40.0)*0.02
	}

	annualDecay := params.DecayCoeff * tempFactor * humidFactor * moistureFactor * ftFactor * params.BiologicalFactor * 100.0

	return math.Max(annualDecay, 0.01)
}

func (a *Assessor) calcRockErosionRate(params RockDecayParams, avgHumid, tempRange float64, cycles int) float64 {
	chemFactor := 0.005 * (avgHumid / 100.0)
	physFactor := 0.003 * (tempRange / 30.0)
	ftFactor := float64(cycles) * 0.0002 * params.PoreWaterFrac * params.FreezePressure

	totalErosion := (chemFactor + physFactor + ftFactor) * 1000.0
	return math.Max(totalErosion, 0.005)
}

func (a *Assessor) gradeWeathering(crackWidth, crackRate float64, cycles int) string {
	score := 0.0
	if crackWidth >= a.cfg.CriticalCrackDepth/10.0 {
		score += 30
	} else if crackWidth >= 3.0 {
		score += 20
	} else if crackWidth >= 1.0 {
		score += 10
	}

	if crackRate >= 0.3 {
		score += 40
	} else if crackRate >= 0.1 {
		score += 25
	} else if crackRate >= 0.05 {
		score += 10
	}

	if cycles >= 500 {
		score += 30
	} else if cycles >= 200 {
		score += 20
	} else if cycles >= 100 {
		score += 10
	}

	switch {
	case score >= 80:
		return "SEVERE"
	case score >= 55:
		return "SERIOUS"
	case score >= 30:
		return "MODERATE"
	case score >= 10:
		return "MILD"
	default:
		return "SLIGHT"
	}
}

func (a *Assessor) estimateCurrentAge(site *models.PlankroadSite) float64 {
	eraAges := map[string]float64{
		"战国":     2400,
		"战国-唐宋":  1500,
		"战国-明清":  1200,
		"秦汉":     2200,
		"秦汉-明清":  1000,
		"三国":     1800,
		"唐代":     1400,
		"唐宋":     1000,
		"明清":     400,
	}
	if age, ok := eraAges[site.ConstructionEra]; ok {
		return age
	}
	return 1000.0
}

func (a *Assessor) predictRockLifespan(crackWidth, growthRate float64) float64 {
	if growthRate <= 0 {
		return a.cfg.RockDesignLife
	}

	criticalDepth := a.cfg.CriticalCrackDepth
	remainingDepth := criticalDepth - crackWidth
	if remainingDepth <= 0 {
		return 1.0
	}

	yearsToCritical := remainingDepth / growthRate

	factor := 1.0
	if crackWidth > 10 {
		factor *= 0.7
	}
	if crackWidth > 25 {
		factor *= 0.5
	}

	return math.Min(yearsToCritical*factor, a.cfg.RockDesignLife)
}

func (a *Assessor) predictWoodLifespan(decayRate, currentAge float64) float64 {
	if decayRate <= 0 {
		return a.cfg.WoodDesignLife
	}

	totalLife := (100.0 / decayRate) * 0.8

	if currentAge >= totalLife {
		return currentAge + 5.0
	}

	return totalLife
}

func (a *Assessor) calcConfidence(dataPoints int, cycles int, stressRange float64) float64 {
	dataScore := math.Min(float64(dataPoints)/500.0, 1.0) * 0.35
	cycleScore := math.Min(float64(cycles)/300.0, 1.0) * 0.25
	stressScore := math.Min(stressRange/20.0, 1.0) * 0.2
	modelScore := 0.2

	total := dataScore + cycleScore + stressScore + modelScore
	return math.Min(math.Max(total, 0.3), 0.98)
}

func getRockParams(name string) RockDecayParams {
	if p, ok := RockParams[name]; ok {
		return p
	}
	return RockDecayParams{K_IC: 1.5, C: 1.0e-12, m: 3.0, PoreWaterFrac: 0.03, FreezePressure: 2.5}
}

func getWoodParams(name string) WoodDecayParams {
	if p, ok := WoodParams[name]; ok {
		return p
	}
	return WoodDecayParams{InitMoisture: 12.0, DecayCoeff: 0.005, TempCoeff: 0.085, HumidCoeff: 0.005, BiologicalFactor: 0.75}
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
