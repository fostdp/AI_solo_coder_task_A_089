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
	CurrentState    FreezeThawState
	Cycles          int
	LastStateChange time.Time
	FreezeDuration  time.Duration
	ThawDuration    time.Duration
	TotalFreezeHrs  float64
	TotalThawHrs    float64
	MaxFreezeRate   float64
	MaxThawRate     float64
}

type PoreStructure struct {
	Porosity       float64
	AvgPoreRadius  float64
	PoreSizeDisp   float64
	Saturation     float64
	Permeability   float64
}

type ThermalProps struct {
	AlphaParallel float64
	AlphaNormal   float64
	Conductivity  float64
	HeatCapacity  float64
	AnisotropyIdx float64
}

type RockDecayParams struct {
	K_IC           float64
	C              float64
	m              float64
	Pore           PoreStructure
	Thermal        ThermalProps
	FreezePressure float64
	FTDamageBeta   float64
	ChemicalFactor float64
	ShockFactor    float64
	BeddingFactor  float64
	RockClass      string
	HardnessMohs   float64
}

type WoodDecayParams struct {
	InitMoisture     float64
	DecayCoeff       float64
	TempCoeff        float64
	HumidCoeff       float64
	BiologicalFactor float64
	LigninContent    float64
	DurabilityClass  int
}

type Assessor struct {
	cfg        *config.WeatherConfig
	ftTrackers map[int]*FreezeThawTracker
}

func NewAssessor(cfg *config.WeatherConfig) *Assessor {
	return &Assessor{
		cfg:        cfg,
		ftTrackers: make(map[int]*FreezeThawTracker),
	}
}

var RockParams = map[string]RockDecayParams{
	"石灰岩": {
		K_IC: 1.5, C: 1.0e-12, m: 3.0,
		Pore:     PoreStructure{Porosity: 0.03, AvgPoreRadius: 50e-9, PoreSizeDisp: 2.0, Saturation: 0.7, Permeability: 1e-15},
		Thermal:  ThermalProps{AlphaParallel: 8.0e-6, AlphaNormal: 8.0e-6, Conductivity: 2.8, HeatCapacity: 850, AnisotropyIdx: 1.0},
		FreezePressure: 2.8, FTDamageBeta: 0.0018, ChemicalFactor: 2.8, ShockFactor: 1.0, BeddingFactor: 1.0,
		RockClass: "carbonate", HardnessMohs: 3.5,
	},
	"花岗岩": {
		K_IC: 2.5, C: 5.0e-13, m: 3.2,
		Pore:     PoreStructure{Porosity: 0.008, AvgPoreRadius: 5e-9, PoreSizeDisp: 3.5, Saturation: 0.35, Permeability: 1e-18},
		Thermal:  ThermalProps{AlphaParallel: 7.5e-6, AlphaNormal: 7.5e-6, Conductivity: 3.2, HeatCapacity: 780, AnisotropyIdx: 1.0},
		FreezePressure: 1.5, FTDamageBeta: 0.0006, ChemicalFactor: 0.4, ShockFactor: 2.5, BeddingFactor: 1.0,
		RockClass: "igneous", HardnessMohs: 6.5,
	},
	"片麻岩": {
		K_IC: 1.8, C: 8.0e-13, m: 3.1,
		Pore:     PoreStructure{Porosity: 0.012, AvgPoreRadius: 15e-9, PoreSizeDisp: 2.8, Saturation: 0.45, Permeability: 5e-17},
		Thermal:  ThermalProps{AlphaParallel: 6.5e-6, AlphaNormal: 10.5e-6, Conductivity: 2.9, HeatCapacity: 800, AnisotropyIdx: 1.6},
		FreezePressure: 2.0, FTDamageBeta: 0.0012, ChemicalFactor: 0.7, ShockFactor: 1.8, BeddingFactor: 2.2,
		RockClass: "metamorphic-foliated", HardnessMohs: 6.0,
	},
	"大理岩": {
		K_IC: 1.3, C: 1.2e-12, m: 2.9,
		Pore:     PoreStructure{Porosity: 0.02, AvgPoreRadius: 30e-9, PoreSizeDisp: 2.2, Saturation: 0.6, Permeability: 5e-16},
		Thermal:  ThermalProps{AlphaParallel: 7.8e-6, AlphaNormal: 8.5e-6, Conductivity: 3.0, HeatCapacity: 830, AnisotropyIdx: 1.1},
		FreezePressure: 2.5, FTDamageBeta: 0.0020, ChemicalFactor: 2.5, ShockFactor: 1.3, BeddingFactor: 1.3,
		RockClass: "carbonate-metamorphic", HardnessMohs: 3.0,
	},
	"砂岩": {
		K_IC: 0.8, C: 2.0e-12, m: 2.8,
		Pore:     PoreStructure{Porosity: 0.10, AvgPoreRadius: 100e-9, PoreSizeDisp: 1.8, Saturation: 0.8, Permeability: 1e-13},
		Thermal:  ThermalProps{AlphaParallel: 7.0e-6, AlphaNormal: 11.0e-6, Conductivity: 2.3, HeatCapacity: 900, AnisotropyIdx: 1.7},
		FreezePressure: 4.0, FTDamageBeta: 0.0028, ChemicalFactor: 1.5, ShockFactor: 1.5, BeddingFactor: 2.8,
		RockClass: "siliciclastic", HardnessMohs: 5.0,
	},
	"板岩": {
		K_IC: 1.0, C: 1.5e-12, m: 2.85,
		Pore:     PoreStructure{Porosity: 0.04, AvgPoreRadius: 25e-9, PoreSizeDisp: 3.0, Saturation: 0.65, Permeability: 1e-15},
		Thermal:  ThermalProps{AlphaParallel: 5.0e-6, AlphaNormal: 13.0e-6, Conductivity: 2.5, HeatCapacity: 880, AnisotropyIdx: 2.6},
		FreezePressure: 3.2, FTDamageBeta: 0.0022, ChemicalFactor: 0.9, ShockFactor: 1.6, BeddingFactor: 3.5,
		RockClass: "metamorphic-schistose", HardnessMohs: 4.0,
	},
}

var WoodParams = map[string]WoodDecayParams{
	"柏木":   {InitMoisture: 12.0, DecayCoeff: 0.004, TempCoeff: 0.08, HumidCoeff: 0.005, BiologicalFactor: 0.7, LigninContent: 0.34, DurabilityClass: 2},
	"青冈木":  {InitMoisture: 10.0, DecayCoeff: 0.003, TempCoeff: 0.07, HumidCoeff: 0.004, BiologicalFactor: 0.6, LigninContent: 0.30, DurabilityClass: 1},
	"松木":   {InitMoisture: 15.0, DecayCoeff: 0.006, TempCoeff: 0.10, HumidCoeff: 0.006, BiologicalFactor: 0.9, LigninContent: 0.27, DurabilityClass: 3},
	"栎木":   {InitMoisture: 11.0, DecayCoeff: 0.0035, TempCoeff: 0.075, HumidCoeff: 0.0045, BiologicalFactor: 0.65, LigninContent: 0.32, DurabilityClass: 2},
	"杉木":   {InitMoisture: 14.0, DecayCoeff: 0.005, TempCoeff: 0.09, HumidCoeff: 0.0055, BiologicalFactor: 0.8, LigninContent: 0.33, DurabilityClass: 2},
}

func (a *Assessor) Assess(site *models.PlankroadSite, readings []models.SensorReading, sim *models.StructuralSimulation) *models.WeatheringAssessment {
	if _, ok := a.ftTrackers[site.SiteID]; !ok {
		a.ftTrackers[site.SiteID] = &FreezeThawTracker{}
	}

	rockParams := getRockParams(site.RockType)
	woodParams := getWoodParams(site.WoodType)

	cycles, freezeStats := a.calcFreezeThawCycles(site.SiteID, readings, rockParams)
	avgTemp, avgHumid, tempRange, minTemp, maxTemp, tempTrans := calcClimateParams(readings)

	avgCrack := 0.0
	maxCrack := 0.0
	if len(readings) > 0 {
		for _, r := range readings {
			avgCrack += r.MaxCrackWidth
			if r.MaxCrackWidth > maxCrack {
				maxCrack = r.MaxCrackWidth
			}
		}
		avgCrack /= float64(len(readings))
	}

	stressRange := 5.0
	if sim != nil {
		stressRange = math.Max(sim.MaxRockStress-sim.MinRockStress, 5.0)
	}

	ftDamage := calcFTDamage(rockParams, cycles, freezeStats)
	shockDamage := calcThermalShock(rockParams, tempRange, float64(tempTrans))
	chemDamage := calcChemicalErosion(rockParams, avgTemp, avgHumid)

	crackPropRate := a.calcCrackPropagation(rockParams, stressRange, avgCrack, cycles, ftDamage, shockDamage, chemDamage)
	crackRate := crackPropRate * 12.0

	beddingAccel := rockParams.BeddingFactor * (1.0 + rockParams.Thermal.AnisotropyIdx*0.1)
	crackPropRate *= math.Pow(beddingAccel, 0.3)
	crackRate = crackPropRate * 12.0

	weatherGrade := a.gradeWeathering(rockParams, maxCrack, crackRate, cycles, ftDamage, shockDamage)

	woodDecay := a.calcWoodDecayRate(woodParams, avgTemp, avgHumid, cycles, freezeStats)
	rockErosion := a.calcRockErosionRate(rockParams, avgHumid, tempRange, cycles, ftDamage, chemDamage)

	currentAge := a.estimateCurrentAge(site)
	ageDamageFactor := 1.0 + currentAge*0.0005*rockParams.ChemicalFactor

	rockLife := a.predictRockLifespan(maxCrack, crackPropRate, rockParams, ftDamage)
	woodLife := a.predictWoodLifespan(woodDecay, currentAge)
	predictedLife := math.Min(rockLife, woodLife)
	predictedLife /= ageDamageFactor

	remaining := math.Max(predictedLife-currentAge, 0.1)
	confidence := a.calcConfidence(len(readings), cycles, stressRange, rockParams)

	freezeDepression := calcFreezingPointDepression(rockParams)

	detail := map[string]interface{}{
		"avg_temperature":          round4(avgTemp),
		"min_temperature":          round4(minTemp),
		"max_temperature":          round4(maxTemp),
		"avg_humidity":             round4(avgHumid),
		"temperature_range":        round4(tempRange),
		"temp_transitions":         tempTrans,
		"avg_crack_width_mm":       round4(avgCrack),
		"max_crack_width_mm":       round4(maxCrack),
		"monthly_crack_rate_mm":    round4(crackRate),
		"rock_fracture_toughness":  rockParams.K_IC,
		"rock_porosity":            rockParams.Pore.Porosity,
		"rock_avg_pore_radius_nm":  rockParams.Pore.AvgPoreRadius * 1e9,
		"rock_permeability":        rockParams.Pore.Permeability,
		"freezing_point_depression": round4(freezeDepression),
		"effective_freeze_temp":    round4(a.cfg.FreezeTemp + freezeDepression),
		"freeze_pressure_mpa":      rockParams.FreezePressure,
		"ft_damage_index":          round4(ftDamage),
		"thermal_shock_index":      round4(shockDamage),
		"chemical_erosion_index":   round4(chemDamage),
		"thermal_anisotropy":       rockParams.Thermal.AnisotropyIdx,
		"bedding_acceleration":     round4(beddingAccel),
		"wood_lignin_content":      woodParams.LigninContent,
		"wood_moisture_content":    round4(woodParams.InitMoisture + avgHumid*0.1),
		"wood_durability_class":    woodParams.DurabilityClass,
		"estimated_age_years":      round4(currentAge),
		"age_damage_factor":        round4(ageDamageFactor),
		"rock_lifespan_years":      round4(rockLife),
		"wood_lifespan_years":      round4(woodLife),
		"rock_hardness_mohs":       rockParams.HardnessMohs,
		"rock_class":               rockParams.RockClass,
		"freeze_duration_hours":    round4(freezeStats.avgFreezeHrs),
		"thaw_duration_hours":      round4(freezeStats.avgThawHrs),
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

type freezeStats struct {
	avgFreezeHrs float64
	avgThawHrs   float64
	maxFreezeRate float64
	maxThawRate   float64
	depthBelowZero float64
}

func (a *Assessor) calcFreezeThawCycles(siteID int, readings []models.SensorReading, rp RockDecayParams) (int, freezeStats) {
	tracker := a.ftTrackers[siteID]
	stats := freezeStats{}

	if len(readings) < 2 {
		return tracker.Cycles + 80, stats
	}

	baseCycles := tracker.Cycles
	baseFreezeTemp := a.cfg.FreezeTemp
	baseThawTemp := a.cfg.ThawTemp

	fpDepression := calcFreezingPointDepression(rp)
	freezeT := baseFreezeTemp + fpDepression
	thawT := baseThawTemp + fpDepression*0.5

	prevTemp := readings[len(readings)-1].Temperature
	prevTime := readings[len(readings)-1].Time
	transitions := 0
	freezingPeriods := []float64{}
	thawingPeriods := []float64{}
	maxFreezeRate := 0.0
	maxThawRate := 0.0
	totalFreezeHrs := 0.0
	currentState := StateNormal
	cycleStart := readings[len(readings)-1].Time

	for i := len(readings) - 2; i >= 0; i-- {
		currTemp := readings[i].Temperature
		currTime := readings[i].Time
		hours := prevTime.Sub(currTime).Hours()
		if hours <= 0 {
			hours = 1.0
		}

		coolingRate := (prevTemp - currTemp) / hours
		if coolingRate > maxFreezeRate {
			maxFreezeRate = coolingRate
		}
		warmingRate := (currTemp - prevTemp) / hours
		if warmingRate > maxThawRate {
			maxThawRate = warmingRate
		}

		if prevTemp > thawT && currTemp < freezeT {
			transitions++
			if currentState != StateFreezing {
				currentState = StateFreezing
				cycleStart = currTime
			}
		} else if prevTemp < freezeT && currTemp > thawT {
			transitions++
			if currentState == StateFreezing {
				dur := cycleStart.Sub(currTime).Hours()
				if dur > 0 {
					freezingPeriods = append(freezingPeriods, dur)
				}
			}
			currentState = StateThawing
			thawingPeriods = append(thawingPeriods, hours)
		}

		if currTemp < freezeT {
			totalFreezeHrs += hours
		}

		prevTemp = currTemp
		prevTime = currTime
	}

	newCycles := transitions / 2
	tracker.Cycles = baseCycles + newCycles
	tracker.TotalFreezeHrs += totalFreezeHrs
	tracker.MaxFreezeRate = math.Max(tracker.MaxFreezeRate, maxFreezeRate)
	tracker.MaxThawRate = math.Max(tracker.MaxThawRate, maxThawRate)

	stats.depthBelowZero = math.Max(0, -math.Min(0, baseFreezeTemp-fpDepression))
	if len(freezingPeriods) > 0 {
		for _, d := range freezingPeriods {
			stats.avgFreezeHrs += d
		}
		stats.avgFreezeHrs /= float64(len(freezingPeriods))
	}
	if len(thawingPeriods) > 0 {
		for _, d := range thawingPeriods {
			stats.avgThawHrs += d
		}
		stats.avgThawHrs /= float64(len(thawingPeriods))
	}
	stats.maxFreezeRate = tracker.MaxFreezeRate
	stats.maxThawRate = tracker.MaxThawRate
	if stats.avgFreezeHrs == 0 {
		stats.avgFreezeHrs = 6.0
	}
	if stats.avgThawHrs == 0 {
		stats.avgThawHrs = 8.0
	}

	return baseCycles + newCycles + 80, stats
}

func calcFreezingPointDepression(rp RockDecayParams) float64 {
	const gammaSL = 0.032
	const latentHeat = 334e3
	const rhoIce = 917.0
	const T0 = 273.15

	rpore := rp.Pore.AvgPoreRadius * rp.Pore.PoreSizeDisp * 0.5
	if rpore < 1e-10 {
		rpore = 1e-9
	}

	deltaT := (2.0 * gammaSL * T0) / (latentHeat * rhoIce * rpore)
	deltaT *= math.Pow(rp.Pore.Saturation, 0.67)

	return math.Min(deltaT, 5.0)
}

func calcFTDamage(rp RockDecayParams, cycles int, fs freezeStats) float64 {
	D := 1.0 - math.Exp(-rp.FTDamageBeta*float64(cycles))

	pressureFactor := 1.0 + (rp.FreezePressure-2.0)*0.3
	porosityFactor := 1.0 + (rp.Pore.Porosity-0.03)*10.0
	saturationFactor := math.Pow(rp.Pore.Saturation, 1.5)
	rateFactor := 1.0 + (fs.maxFreezeRate+fs.maxThawRate)*0.01
	depthFactor := 1.0 + fs.depthBelowZero*0.15

	totalFactor := pressureFactor * porosityFactor * saturationFactor * rateFactor * depthFactor
	return math.Min(D*totalFactor, 0.99)
}

func calcThermalShock(rp RockDecayParams, tempRange, tempTrans float64) float64 {
	shockBase := 1.0 - math.Exp(-rp.ShockFactor*0.0004*tempTrans)
	deltaT := math.Max(tempRange, 0.0)
	deltaTc := 20.0 * (2.5 / rp.HardnessMohs)
	shockThermal := math.Pow(deltaT/deltaTc, 1.5)
	anisotropy := math.Pow(rp.Thermal.AnisotropyIdx, 0.8)

	total := (shockBase*0.3 + shockThermal*0.7) * anisotropy
	return math.Min(total, 0.9)
}

func calcChemicalErosion(rp RockDecayParams, avgTemp, avgHumid float64) float64 {
	tempFactor := math.Exp(0.08 * (avgTemp - 15.0))
	humidFactor := math.Pow(avgHumid/100.0, 1.3)

	carbonateFactor := 1.0
	if rp.RockClass == "carbonate" || rp.RockClass == "carbonate-metamorphic" {
		carbonateFactor = rp.ChemicalFactor * (1.0 + (avgHumid-70.0)*0.01)
	} else {
		carbonateFactor = rp.ChemicalFactor
	}

	permFactor := 1.0 + math.Log10(math.Max(rp.Pore.Permeability, 1e-20))*(-0.05)

	total := 0.01 * tempFactor * humidFactor * carbonateFactor * permFactor
	return math.Min(total, 0.5)
}

func calcClimateParams(readings []models.SensorReading) (avgTemp, avgHumid, tempRange, minTemp, maxTemp float64, transitions int) {
	if len(readings) == 0 {
		return 15.0, 70.0, 25.0, 5.0, 25.0, 50
	}

	minTemp, maxTemp = 1e10, -1e10
	prevT := 0.0
	for i, r := range readings {
		avgTemp += r.Temperature
		avgHumid += r.Humidity
		if r.Temperature < minTemp {
			minTemp = r.Temperature
		}
		if r.Temperature > maxTemp {
			maxTemp = r.Temperature
		}
		if i > 0 {
			if (prevT > 0 && r.Temperature <= 0) || (prevT <= 0 && r.Temperature > 0) {
				transitions++
			}
		}
		prevT = r.Temperature
	}
	n := float64(len(readings))
	avgTemp /= n
	avgHumid /= n
	tempRange = math.Max(maxTemp-minTemp, 15.0)
	transitions = int(math.Max(float64(transitions), float64(len(readings))*0.1))
	return
}

func (a *Assessor) calcCrackPropagation(params RockDecayParams, stressRange, crackWidth float64,
	cycles int, ftDamage, shockDamage, chemDamage float64) float64 {
	pi := math.Pi
	geoFactor := 1.12 * math.Sqrt(params.BeddingFactor)

	if crackWidth <= 0.01 {
		crackWidth = 0.1
	}

	deltaK := geoFactor * stressRange * math.Sqrt(pi*crackWidth/1000.0)

	if deltaK >= params.K_IC {
		return a.cfg.CriticalCrackDepth / 50.0
	}

	waterIceFactor := 1.0
	if params.Pore.Porosity > 0.05 {
		waterIceFactor = 1.0 + 9.0*(params.Pore.Porosity-0.05)
	}

	environFactor := (1.0 + ftDamage*3.0) * (1.0 + shockDamage*2.0) * (1.0 + chemDamage*1.5) * waterIceFactor

	da_dN := params.C * math.Pow(deltaK, params.m) * environFactor

	annualCycles := math.Max(float64(cycles), 80.0)
	annualGrowth := da_dN * 1e3 * annualCycles

	minRate := 0.001 + params.Pore.Porosity*0.08 + ftDamage*0.01
	return math.Max(annualGrowth, minRate)
}

func (a *Assessor) calcWoodDecayRate(params WoodDecayParams, avgTemp, avgHumid float64, cycles int, fs freezeStats) float64 {
	tempFactor := math.Exp(params.TempCoeff * (avgTemp - 20.0))
	humidFactor := 1.0 + params.HumidCoeff*(avgHumid-65.0)
	if humidFactor < 0.1 {
		humidFactor = 0.1
	}

	ftFactor := 1.0 + float64(cycles)*0.001*(1.0+(1.0-float64(params.DurabilityClass)*0.2))

	moisture := params.InitMoisture + avgHumid*0.08 + fs.avgFreezeHrs*0.05

	var moistureFactor float64
	if moisture < 20 {
		moistureFactor = 0.1 + (moisture / 20.0) * 0.5
	} else if moisture <= 40 {
		moistureFactor = 0.6 + ((moisture - 20.0) / 20.0) * 0.4
	} else {
		moistureFactor = 1.0 + (moisture-40.0)*0.02
	}

	ligninFactor := 1.0 + (0.35-params.LigninContent)*3.0
	durabilityFactor := math.Pow(1.5, float64(3-params.DurabilityClass))

	annualDecay := params.DecayCoeff * tempFactor * humidFactor * moistureFactor *
		ftFactor * params.BiologicalFactor * ligninFactor * durabilityFactor * 100.0

	return math.Max(annualDecay, 0.01)
}

func (a *Assessor) calcRockErosionRate(params RockDecayParams, avgHumid, tempRange float64,
	cycles int, ftDamage, chemDamage float64) float64 {
	physFactor := 0.003 * (tempRange / 30.0) * (1.0 + ftDamage*2.0)
	chemFactor := 0.005 * (avgHumid / 100.0) * (1.0 + chemDamage*3.0)
	ftFactor := float64(cycles) * 0.0002 * params.Pore.Porosity * params.FreezePressure * (1.0 + ftDamage)
	shockFactor := 0.001 * params.ShockFactor * tempRange / 20.0

	hardnessFactor := 1.0 / math.Max(params.HardnessMohs/4.0, 0.6)
	beddingFactor := math.Sqrt(params.BeddingFactor)

	totalErosion := (physFactor + chemFactor + ftFactor + shockFactor) * 1000.0 * hardnessFactor * beddingFactor
	return math.Max(totalErosion, 0.005)
}

func (a *Assessor) gradeWeathering(rp RockDecayParams, crackWidth, crackRate float64, cycles int, ftD, shockD float64) string {
	baseRate := 0.05 / math.Sqrt(rp.HardnessMohs)
	score := 0.0

	criticalNorm := a.cfg.CriticalCrackDepth / 10.0
	if crackWidth >= criticalNorm {
		score += 30
	} else if crackWidth >= criticalNorm*0.6 {
		score += 20
	} else if crackWidth >= criticalNorm*0.2 {
		score += 10
	}

	if crackRate >= baseRate*6 {
		score += 40
	} else if crackRate >= baseRate*2 {
		score += 25
	} else if crackRate >= baseRate {
		score += 10
	}

	if cycles >= 500 {
		score += 15
	} else if cycles >= 200 {
		score += 10
	} else if cycles >= 100 {
		score += 5
	}

	score += ftD * 25
	score += shockD * 15

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
		"战国":    2400,
		"战国-唐宋": 1500,
		"战国-明清": 1200,
		"秦汉":    2200,
		"秦汉-明清": 1000,
		"三国":    1800,
		"唐代":    1400,
		"唐宋":    1000,
		"明清":    400,
	}
	if age, ok := eraAges[site.ConstructionEra]; ok {
		return age
	}
	return 1000.0
}

func (a *Assessor) predictRockLifespan(crackWidth, growthRate float64, rp RockDecayParams, ftDamage float64) float64 {
	if growthRate <= 0 {
		return a.cfg.RockDesignLife
	}

	criticalDepth := a.cfg.CriticalCrackDepth / math.Sqrt(rp.BeddingFactor)
	remainingDepth := criticalDepth - crackWidth
	if remainingDepth <= 0 {
		return 1.0
	}

	acceleratedRate := growthRate * (1.0 + ftDamage*1.5)
	yearsToCritical := remainingDepth / acceleratedRate

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

func (a *Assessor) calcConfidence(dataPoints int, cycles int, stressRange float64, rp RockDecayParams) float64 {
	dataScore := math.Min(float64(dataPoints)/500.0, 1.0) * 0.3
	cycleScore := math.Min(float64(cycles)/300.0, 1.0) * 0.2
	stressScore := math.Min(stressRange/20.0, 1.0) * 0.15
	rockModelScore := 0.15 + 0.1*(1.0-1.0/rp.Thermal.AnisotropyIdx)
	physicsScore := 0.15

	total := dataScore + cycleScore + stressScore + rockModelScore + physicsScore
	return math.Min(math.Max(total, 0.3), 0.98)
}

func getRockParams(name string) RockDecayParams {
	if p, ok := RockParams[name]; ok {
		return p
	}
	return RockDecayParams{
		K_IC: 1.5, C: 1.0e-12, m: 3.0,
		Pore:     PoreStructure{Porosity: 0.03, AvgPoreRadius: 50e-9, PoreSizeDisp: 2.0, Saturation: 0.65, Permeability: 1e-15},
		Thermal:  ThermalProps{AlphaParallel: 7.5e-6, AlphaNormal: 9.0e-6, Conductivity: 2.7, HeatCapacity: 850, AnisotropyIdx: 1.2},
		FreezePressure: 2.5, FTDamageBeta: 0.0015, ChemicalFactor: 1.2, ShockFactor: 1.5, BeddingFactor: 1.5,
		RockClass: "generic", HardnessMohs: 4.5,
	}
}

func getWoodParams(name string) WoodDecayParams {
	if p, ok := WoodParams[name]; ok {
		return p
	}
	return WoodParams{InitMoisture: 12.0, DecayCoeff: 0.005, TempCoeff: 0.085, HumidCoeff: 0.005, BiologicalFactor: 0.75, LigninContent: 0.30, DurabilityClass: 2}
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
