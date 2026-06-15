package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"plankroad-backend/database"
	"plankroad-backend/models"
)

type SensorRepo struct{}

func NewSensorRepo() *SensorRepo { return &SensorRepo{} }

func (r *SensorRepo) InsertReading(ctx context.Context, s *models.SensorReading) error {
	query := `
		INSERT INTO sensor_readings (
			time, site_id, beam_id,
			beam_strain_top, beam_strain_bottom, beam_strain_side,
			rock_crack_width_1, rock_crack_width_2, rock_crack_width_3,
			temperature, humidity, rainfall
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`
	_, err := database.Pool.Exec(ctx, query,
		s.Time, s.SiteID, s.BeamID,
		s.BeamStrainTop, s.BeamStrainBottom, s.BeamStrainSide,
		s.RockCrackWidth1, s.RockCrackWidth2, s.RockCrackWidth3,
		s.Temperature, s.Humidity, s.Rainfall,
	)
	return err
}

func (r *SensorRepo) BatchInsert(ctx context.Context, readings []models.SensorReading) error {
	rows := [][]interface{}{}
	for _, s := range readings {
		rows = append(rows, []interface{}{
			s.Time, s.SiteID, s.BeamID,
			s.BeamStrainTop, s.BeamStrainBottom, s.BeamStrainSide,
			s.RockCrackWidth1, s.RockCrackWidth2, s.RockCrackWidth3,
			s.Temperature, s.Humidity, s.Rainfall,
		})
	}
	_, err := database.Pool.CopyFrom(
		ctx,
		pgx.Identifier{"sensor_readings"},
		[]string{
			"time", "site_id", "beam_id",
			"beam_strain_top", "beam_strain_bottom", "beam_strain_side",
			"rock_crack_width_1", "rock_crack_width_2", "rock_crack_width_3",
			"temperature", "humidity", "rainfall",
		},
		pgx.CopyFromRows(rows),
	)
	return err
}

func (r *SensorRepo) GetBySite(ctx context.Context, siteID int, start, end time.Time, limit int) ([]models.SensorReading, error) {
	query := `
		SELECT time, site_id, beam_id,
			beam_strain_top, beam_strain_bottom, beam_strain_side,
			rock_crack_width_1, rock_crack_width_2, rock_crack_width_3,
			temperature, humidity, rainfall,
			avg_strain, max_crack_width, strain_alarm, crack_alarm
		FROM sensor_readings
		WHERE site_id = $1 AND time BETWEEN $2 AND $3
		ORDER BY time DESC
		LIMIT $4
	`
	rows, err := database.Pool.Query(ctx, query, siteID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SensorReading
	for rows.Next() {
		var s models.SensorReading
		err := rows.Scan(
			&s.Time, &s.SiteID, &s.BeamID,
			&s.BeamStrainTop, &s.BeamStrainBottom, &s.BeamStrainSide,
			&s.RockCrackWidth1, &s.RockCrackWidth2, &s.RockCrackWidth3,
			&s.Temperature, &s.Humidity, &s.Rainfall,
			&s.AvgStrain, &s.MaxCrackWidth, &s.StrainAlarm, &s.CrackAlarm,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func (r *SensorRepo) GetDailySummary(ctx context.Context, siteID int, days int) ([]models.DailySummary, error) {
	query := `
		SELECT site_id, bucket_day, reading_count,
			avg_daily_strain, max_daily_strain,
			avg_daily_crack, max_daily_crack,
			avg_temp, min_temp, max_temp,
			avg_humidity, alarm_count
		FROM sensor_daily_summary
		WHERE site_id = $1 AND bucket_day >= NOW() - $2::interval
		ORDER BY bucket_day DESC
	`
	rows, err := database.Pool.Query(ctx, query, siteID, fmt.Sprintf("%d days", days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.DailySummary
	for rows.Next() {
		var d models.DailySummary
		err := rows.Scan(
			&d.SiteID, &d.BucketDay, &d.ReadingCount,
			&d.AvgDailyStrain, &d.MaxDailyStrain,
			&d.AvgDailyCrack, &d.MaxDailyCrack,
			&d.AvgTemp, &d.MinTemp, &d.MaxTemp,
			&d.AvgHumidity, &d.AlarmCount,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

type SiteRepo struct{}

func NewSiteRepo() *SiteRepo { return &SiteRepo{} }

func (r *SiteRepo) GetAll(ctx context.Context) ([]models.PlankroadSite, error) {
	rows, err := database.Pool.Query(ctx, `
		SELECT site_id, site_name, region, elevation, construction_era,
			   total_length, beam_count, rock_type, wood_type, created_at
		FROM plankroad_sites ORDER BY site_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sites []models.PlankroadSite
	for rows.Next() {
		var s models.PlankroadSite
		err := rows.Scan(&s.SiteID, &s.SiteName, &s.Region, &s.Elevation,
			&s.ConstructionEra, &s.TotalLength, &s.BeamCount,
			&s.RockType, &s.WoodType, &s.CreatedAt)
		if err != nil {
			return nil, err
		}
		sites = append(sites, s)
	}
	return sites, rows.Err()
}

func (r *SiteRepo) GetByID(ctx context.Context, id int) (*models.PlankroadSite, error) {
	var s models.PlankroadSite
	err := database.Pool.QueryRow(ctx, `
		SELECT site_id, site_name, region, elevation, construction_era,
			   total_length, beam_count, rock_type, wood_type, created_at
		FROM plankroad_sites WHERE site_id = $1
	`, id).Scan(&s.SiteID, &s.SiteName, &s.Region, &s.Elevation,
		&s.ConstructionEra, &s.TotalLength, &s.BeamCount,
		&s.RockType, &s.WoodType, &s.CreatedAt)
	return &s, err
}

type SimulationRepo struct{}

func NewSimulationRepo() *SimulationRepo { return &SimulationRepo{} }

func (r *SimulationRepo) Save(ctx context.Context, sim *models.StructuralSimulation) error {
	query := `
		INSERT INTO structural_simulation (
			site_id, sim_time,
			wood_elastic_modulus, rock_elastic_modulus,
			wood_poisson_ratio, rock_poisson_ratio,
			dead_load, live_load, thermal_load,
			max_wood_stress, min_wood_stress,
			max_rock_stress, min_rock_stress,
			max_deflection_mm, safety_factor, element_data
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING sim_id
	`
	return database.Pool.QueryRow(ctx, query,
		sim.SiteID, sim.SimTime,
		sim.WoodElasticModulus, sim.RockElasticModulus,
		sim.WoodPoissonRatio, sim.RockPoissonRatio,
		sim.DeadLoad, sim.LiveLoad, sim.ThermalLoad,
		sim.MaxWoodStress, sim.MinWoodStress,
		sim.MaxRockStress, sim.MinRockStress,
		sim.MaxDeflectionMM, sim.SafetyFactor, sim.ElementData,
	).Scan(&sim.SimID)
}

func (r *SimulationRepo) GetLatest(ctx context.Context, siteID int) (*models.StructuralSimulation, error) {
	var s models.StructuralSimulation
	err := database.Pool.QueryRow(ctx, `
		SELECT sim_id, site_id, sim_time,
			wood_elastic_modulus, rock_elastic_modulus,
			wood_poisson_ratio, rock_poisson_ratio,
			dead_load, live_load, thermal_load,
			max_wood_stress, min_wood_stress,
			max_rock_stress, min_rock_stress,
			max_deflection_mm, safety_factor, element_data, created_at
		FROM structural_simulation WHERE site_id = $1
		ORDER BY sim_time DESC LIMIT 1
	`, siteID).Scan(&s.SimID, &s.SiteID, &s.SimTime,
		&s.WoodElasticModulus, &s.RockElasticModulus,
		&s.WoodPoissonRatio, &s.RockPoissonRatio,
		&s.DeadLoad, &s.LiveLoad, &s.ThermalLoad,
		&s.MaxWoodStress, &s.MinWoodStress,
		&s.MaxRockStress, &s.MinRockStress,
		&s.MaxDeflectionMM, &s.SafetyFactor, &s.ElementData, &s.CreatedAt)
	return &s, err
}

type WeatheringRepo struct{}

func NewWeatheringRepo() *WeatheringRepo { return &WeatheringRepo{} }

func (r *WeatheringRepo) Save(ctx context.Context, w *models.WeatheringAssessment) error {
	query := `
		INSERT INTO weathering_assessment (
			site_id, assess_time,
			freeze_thaw_cycles, current_crack_depth, crack_propagation_rate,
			weathering_grade, wood_decay_rate, rock_erosion_rate,
			predicted_lifespan, remaining_lifespan, confidence, detail_data
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING assess_id
	`
	return database.Pool.QueryRow(ctx, query,
		w.SiteID, w.AssessTime,
		w.FreezeThawCycles, w.CurrentCrackDepth, w.CrackPropagationRate,
		w.WeatheringGrade, w.WoodDecayRate, w.RockErosionRate,
		w.PredictedLifespan, w.RemainingLifespan, w.Confidence, w.DetailData,
	).Scan(&w.AssessID)
}

func (r *WeatheringRepo) GetLatest(ctx context.Context, siteID int) (*models.WeatheringAssessment, error) {
	var w models.WeatheringAssessment
	err := database.Pool.QueryRow(ctx, `
		SELECT assess_id, site_id, assess_time,
			freeze_thaw_cycles, current_crack_depth, crack_propagation_rate,
			weathering_grade, wood_decay_rate, rock_erosion_rate,
			predicted_lifespan, remaining_lifespan, confidence, detail_data, created_at
		FROM weathering_assessment WHERE site_id = $1
		ORDER BY assess_time DESC LIMIT 1
	`, siteID).Scan(&w.AssessID, &w.SiteID, &w.AssessTime,
		&w.FreezeThawCycles, &w.CurrentCrackDepth, &w.CrackPropagationRate,
		&w.WeatheringGrade, &w.WoodDecayRate, &w.RockErosionRate,
		&w.PredictedLifespan, &w.RemainingLifespan, &w.Confidence, &w.DetailData, &w.CreatedAt)
	return &w, err
}

type AlarmRepo struct{}

func NewAlarmRepo() *AlarmRepo { return &AlarmRepo{} }

func (r *AlarmRepo) GetUnresolved(ctx context.Context, siteID int) ([]models.AlarmEvent, error) {
	query := `
		SELECT alarm_id, time, site_id, beam_id, alarm_type, alarm_level,
			metric_name, current_value, threshold_value, description,
			acknowledged, resolved, mqtt_published, created_at
		FROM alarm_events
		WHERE resolved = FALSE AND ($1 = 0 OR site_id = $1)
		ORDER BY time DESC LIMIT 100
	`
	rows, err := database.Pool.Query(ctx, query, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alarms []models.AlarmEvent
	for rows.Next() {
		var a models.AlarmEvent
		err := rows.Scan(&a.AlarmID, &a.Time, &a.SiteID, &a.BeamID,
			&a.AlarmType, &a.AlarmLevel, &a.MetricName, &a.CurrentValue,
			&a.ThresholdValue, &a.Description, &a.Acknowledged,
			&a.Resolved, &a.MQTTPublished, &a.CreatedAt)
		if err != nil {
			return nil, err
		}
		alarms = append(alarms, a)
	}
	return alarms, rows.Err()
}

func (r *AlarmRepo) Acknowledge(ctx context.Context, alarmID int64, user string) error {
	_, err := database.Pool.Exec(ctx, `
		UPDATE alarm_events SET acknowledged = TRUE, ack_time = NOW(), ack_user = $1
		WHERE alarm_id = $2
	`, user, alarmID)
	return err
}

func (r *AlarmRepo) Resolve(ctx context.Context, alarmID int64) error {
	_, err := database.Pool.Exec(ctx, `
		UPDATE alarm_events SET resolved = TRUE, resolve_time = NOW()
		WHERE alarm_id = $1
	`, alarmID)
	return err
}

func (r *AlarmRepo) MarkMQTTPublished(ctx context.Context, alarmID int64) error {
	_, err := database.Pool.Exec(ctx, `
		UPDATE alarm_events SET mqtt_published = TRUE WHERE alarm_id = $1
	`, alarmID)
	return err
}

func (r *AlarmRepo) GetUnpublished(ctx context.Context) ([]models.AlarmEvent, error) {
	rows, err := database.Pool.Query(ctx, `
		SELECT alarm_id, time, site_id, beam_id, alarm_type, alarm_level,
			metric_name, current_value, threshold_value, description,
			acknowledged, resolved, mqtt_published, created_at
		FROM alarm_events
		WHERE mqtt_published = FALSE
		ORDER BY time ASC LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alarms []models.AlarmEvent
	for rows.Next() {
		var a models.AlarmEvent
		err := rows.Scan(&a.AlarmID, &a.Time, &a.SiteID, &a.BeamID,
			&a.AlarmType, &a.AlarmLevel, &a.MetricName, &a.CurrentValue,
			&a.ThresholdValue, &a.Description, &a.Acknowledged,
			&a.Resolved, &a.MQTTPublished, &a.CreatedAt)
		if err != nil {
			return nil, err
		}
		alarms = append(alarms, a)
	}
	return alarms, rows.Err()
}

func (r *AlarmRepo) GetThresholds(ctx context.Context, siteID int) (*models.AlarmThreshold, error) {
	var t models.AlarmThreshold
	err := database.Pool.QueryRow(ctx, `
		SELECT threshold_id, site_id,
			strain_warning, strain_critical,
			crack_warning, crack_critical,
			crack_rate_warning, crack_rate_critical,
			sf_warning, sf_critical
		FROM alarm_thresholds
		WHERE (site_id = $1 OR site_id IS NULL) AND is_active = TRUE
		ORDER BY site_id NULLS LAST LIMIT 1
	`, siteID).Scan(&t.ThresholdID, &t.SiteID,
		&t.StrainWarning, &t.StrainCritical,
		&t.CrackWarning, &t.CrackCritical,
		&t.CrackRateWarning, &t.CrackRateCritical,
		&t.SFWarning, &t.SFCritical)
	return &t, err
}
