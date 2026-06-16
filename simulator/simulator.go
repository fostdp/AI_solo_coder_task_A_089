package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type RockConfig struct {
	RockType          string  `json:"rock_type"`
	Porosity          float64 `json:"porosity"`
	ThermalExpansion  float64 `json:"thermal_expansion"`
	FractureToughness float64 `json:"fracture_toughness"`
	CrackGrowthRate   float64 `json:"crack_growth_rate"`
	StrainInfluence   float64 `json:"strain_influence"`
	FreezeThawFactor  float64 `json:"freeze_thaw_factor"`
}

type ClimateConfig struct {
	ClimateType    string  `json:"climate_type"`
	TempBase       float64 `json:"temp_base"`
	TempAmplitude  float64 `json:"temp_amplitude"`
	HumidityBase   float64 `json:"humidity_base"`
	RainFactor     float64 `json:"rain_factor"`
	FreezeDays     float64 `json:"freeze_days"`
	DegradeFactor  float64 `json:"degrade_factor"`
}

var RockConfigs = map[string]RockConfig{
	"limestone":  {"limestone", 0.03, 8.0e-6, 1.5, 1.0e-12, 1.0, 1.2},
	"sandstone":  {"sandstone", 0.15, 11.0e-6, 1.2, 2.5e-12, 1.3, 1.8},
	"granite":    {"granite", 0.01, 7.5e-6, 2.0, 0.5e-12, 0.8, 0.7},
	"gneiss":     {"gneiss", 0.02, 8.5e-6, 1.8, 0.8e-12, 0.9, 0.9},
	"shale":      {"shale", 0.10, 12.0e-6, 0.9, 4.0e-12, 1.5, 2.0},
	"quartzite":  {"quartzite", 0.005, 10.0e-6, 2.5, 0.3e-12, 0.7, 0.5},
}

var ClimateConfigs = map[string]ClimateConfig{
	"temperate":   {"temperate", 15.0, 12.0, 70.0, 1.0, 30, 1.0},
	"subtropical": {"subtropical", 20.0, 8.0, 80.0, 1.5, 10, 1.3},
	"alpine":      {"alpine", 5.0, 18.0, 60.0, 0.8, 120, 1.5},
	"arid":        {"arid", 12.0, 15.0, 40.0, 0.3, 40, 0.8},
}

type SiteConfig struct {
	SiteID        int
	SiteName      string
	Elevation     float64
	BeamCount     int
	BaseTemp      float64
	TempAmp       float64
	HumidityBase  float64
	StrainBase    float64
	CrackBase     float64
	RockType      string
	RainFactor    float64
	DegradeFactor float64
	RockConfig    *RockConfig
	ClimateConfig *ClimateConfig
}

var DefaultSites = []SiteConfig{
	{1, "明月峡古栈道", 485.5, 486, 16.5, 12.0, 75.0, 350.0, 1.2, "limestone", 1.3, 1.1, nil, nil},
	{2, "石门栈道",    620.0, 365, 14.5, 14.0, 70.0, 420.0, 0.9, "granite", 1.1, 0.9, nil, nil},
	{3, "子午道遗址",   890.0, 782, 12.0, 16.0, 65.0, 280.0, 1.8, "gneiss", 0.9, 1.3, nil, nil},
	{4, "褒斜道遗址",   750.0, 1098, 13.0, 15.0, 68.0, 310.0, 1.5, "limestone", 1.0, 1.2, nil, nil},
	{5, "陈仓道遗址",   920.0, 695, 11.5, 17.0, 62.0, 380.0, 2.1, "limestone", 0.8, 1.4, nil, nil},
	{6, "金牛道遗址",   540.0, 925, 17.0, 10.0, 78.0, 340.0, 1.6, "sandstone", 1.4, 1.5, nil, nil},
	{7, "米仓道遗址",   1100.0, 538, 9.5, 18.0, 72.0, 260.0, 2.5, "shale", 1.2, 1.6, nil, nil},
	{8, "傥骆道遗址",   1350.0, 442, 7.0, 20.0, 60.0, 450.0, 3.0, "gneiss", 0.7, 1.7, nil, nil},
	{9, "荔枝道遗址",   680.0, 298, 15.5, 13.0, 76.0, 290.0, 1.7, "sandstone", 1.3, 1.3, nil, nil},
	{10, "阴平道遗址",  1450.0, 225, 6.0, 22.0, 58.0, 520.0, 3.5, "limestone", 0.6, 1.8, nil, nil},
}

type SensorReading struct {
	Time             string  `json:"time"`
	SiteID           int     `json:"site_id"`
	BeamID           int     `json:"beam_id"`
	BeamStrainTop    float64 `json:"beam_strain_top"`
	BeamStrainBottom float64 `json:"beam_strain_bottom"`
	BeamStrainSide   float64 `json:"beam_strain_side"`
	RockCrackWidth1  float64 `json:"rock_crack_width_1"`
	RockCrackWidth2  float64 `json:"rock_crack_width_2"`
	RockCrackWidth3  float64 `json:"rock_crack_width_3"`
	Temperature      float64 `json:"temperature"`
	Humidity         float64 `json:"humidity"`
	Rainfall         float64 `json:"rainfall"`
	RockType         string  `json:"rock_type,omitempty"`
	ClimateType      string  `json:"climate_type,omitempty"`
}

var (
	apiURL         = flag.String("api", "http://localhost:8080/api/sensor/batch", "Backend API endpoint")
	mqttBroker     = flag.String("mqtt", "tcp://localhost:1883", "MQTT broker address")
	mqttUser       = flag.String("mqtt-user", "", "MQTT username")
	mqttPass       = flag.String("mqtt-pass", "", "MQTT password")
	mqttTopic      = flag.String("mqtt-topic", "plankroad/data/", "MQTT topic prefix")
	interval       = flag.Duration("interval", 1*time.Hour, "Reporting interval (use shorter for testing)")
	beamsPerSite   = flag.Int("beams", 5, "Beams per site to simulate")
	backfill       = flag.Int("backfill", 0, "Hours of historical data to generate on start")
	useMQTT        = flag.Bool("mqtt-enable", false, "Publish via MQTT instead of HTTP")
	rockType       = flag.String("rock-type", "", "Override rock type for all sites (limestone/sandstone/granite/gneiss/shale/quartzite)")
	climate        = flag.String("climate", "", "Override climate for all sites (temperate/subtropical/alpine/arid)")
	anomalyChance  = flag.Float64("anomaly", 0.05, "Chance of anomaly injection (0-1)")
	siteIDs        = flag.String("sites", "1,2,3,4,5,6,7,8,9,10", "Comma-separated site IDs to simulate")
	mqttClient     mqtt.Client
	startHour      = float64(0)
	activeSites    []SiteConfig
	currentRock    *RockConfig
	currentClimate *ClimateConfig
)

func main() {
	flag.Parse()
	loadEnvConfig()
	rand.Seed(time.Now().UnixNano())

	applyOverrides()

	log.Println("===========================================")
	log.Println("古代栈道传感器模拟器启动")
	log.Printf("启用遗址: %d 处", len(activeSites))
	log.Printf("每遗址梁数: %d", *beamsPerSite)
	log.Printf("上报间隔: %v", *interval)
	log.Printf("后端API: %s", *apiURL)
	log.Printf("MQTT启用: %v", *useMQTT)
	if *useMQTT {
		log.Printf("MQTT Broker: %s", *mqttBroker)
		log.Printf("MQTT Topic: %s", *mqttTopic)
		if err := setupMQTT(); err != nil {
			log.Printf("MQTT setup warning: %v", err)
		}
	}
	if currentRock != nil {
		log.Printf("岩性配置: %s (孔隙率:%.2f%%, 断裂韧性:%.1f MPa√m)",
			currentRock.RockType, currentRock.Porosity*100, currentRock.FractureToughness)
	}
	if currentClimate != nil {
		log.Printf("气候配置: %s (年均温:%.1f°C, 湿度:%.0f%%, 年冻融:%.0f天)",
			currentClimate.ClimateType, currentClimate.TempBase, currentClimate.HumidityBase, currentClimate.FreezeDays)
	}
	log.Printf("异常注入概率: %.1f%%", *anomalyChance*100)
	log.Println("===========================================")

	if *backfill > 0 {
		log.Printf("生成历史数据: %d 小时...", *backfill)
		generateBackfill(*backfill)
		log.Printf("历史数据生成完成!")
	}

	go runScheduler()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("模拟器退出")
}

func loadEnvConfig() {
	if v := os.Getenv("BACKEND_URL"); v != "" {
		*apiURL = v + "/api/sensor/batch"
	}
	if v := os.Getenv("MQTT_BROKER"); v != "" {
		*mqttBroker = v
		*useMQTT = true
	}
	if v := os.Getenv("MQTT_USERNAME"); v != "" {
		*mqttUser = v
	}
	if v := os.Getenv("MQTT_PASSWORD"); v != "" {
		*mqttPass = v
	}
	if v := os.Getenv("MQTT_DATA_TOPIC"); v != "" {
		*mqttTopic = v
	}
	if v := os.Getenv("SIM_INTERVAL_SEC"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil {
			*interval = time.Duration(sec) * time.Second
		}
	}
	if v := os.Getenv("SIM_SITES"); v != "" {
		*siteIDs = v
	}
	if v := os.Getenv("SIM_ROCK_TYPE"); v != "" {
		*rockType = v
	}
	if v := os.Getenv("SIM_CLIMATE"); v != "" {
		*climate = v
	}
	if v := os.Getenv("SIM_ANOMALY_CHANCE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			*anomalyChance = f
		}
	}
}

func applyOverrides() {
	activeSites = make([]SiteConfig, 0, len(DefaultSites))
	enabled := make(map[int]bool)
	for _, s := range strings.Split(*siteIDs, ",") {
		if id, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			enabled[id] = true
		}
	}

	for _, site := range DefaultSites {
		if enabled[site.SiteID] {
			s := site
			if *rockType != "" {
				if rc, ok := RockConfigs[strings.ToLower(*rockType)]; ok {
					s.RockConfig = &rc
					s.RockType = rc.RockType
					currentRock = &rc
				}
			} else if rc, ok := RockConfigs[site.RockType]; ok {
				s.RockConfig = &rc
			}
			if *climate != "" {
				if cc, ok := ClimateConfigs[strings.ToLower(*climate)]; ok {
					s.ClimateConfig = &cc
					s.BaseTemp = cc.TempBase
					s.TempAmp = cc.TempAmplitude
					s.HumidityBase = cc.HumidityBase
					s.RainFactor = cc.RainFactor
					s.DegradeFactor = cc.DegradeFactor
					currentClimate = &cc
				}
			} else {
				cc := ClimateConfigs["temperate"]
				s.ClimateConfig = &cc
			}
			activeSites = append(activeSites, s)
		}
	}
}

func setupMQTT() error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(*mqttBroker)
	opts.SetClientID(fmt.Sprintf("plankroad_simulator_%d", rand.Intn(10000)))
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	if *mqttUser != "" {
		opts.SetUsername(*mqttUser)
	}
	if *mqttPass != "" {
		opts.SetPassword(*mqttPass)
	}

	mqttClient = mqtt.NewClient(opts)
	token := mqttClient.Connect()
	token.WaitTimeout(15 * time.Second)
	return token.Error()
}

func runScheduler() {
	sendReadings(time.Now())

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for t := range ticker.C {
		sendReadings(t)
	}
}

func generateBackfill(hours int) {
	batchSize := 200
	totalHours := hours
	sent := 0

	for h := totalHours; h > 0; h -= batchSize {
		step := batchSize
		if h < step {
			step = h
		}
		batch := make([]SensorReading, 0, step*len(activeSites)*(*beamsPerSite))

		for hi := 0; hi < step; hi++ {
			t := time.Now().Add(-time.Duration(h-hi) * time.Hour)
			for _, site := range activeSites {
				for beam := 1; beam <= *beamsPerSite; beam++ {
					batch = append(batch, generateReading(site, beam, t, float64(totalHours-h+hi)))
				}
			}
		}

		sendBatch(batch)
		sent += step
		log.Printf("历史数据进度: %d/%d 小时", sent, totalHours)
		time.Sleep(200 * time.Millisecond)
	}
}

func sendReadings(t time.Time) {
	batch := make([]SensorReading, 0, len(activeSites)*(*beamsPerSite))
	startHour += (*interval).Hours()

	for _, site := range activeSites {
		for beam := 1; beam <= *beamsPerSite; beam++ {
			batch = append(batch, generateReading(site, beam, t, startHour))
		}
	}

	sendBatch(batch)
	log.Printf("[%s] 上报完成: %d 条数据 (%d遗址x%d梁) [岩性:%s 气候:%s]",
		t.Format("2006-01-02 15:04"), len(batch), len(activeSites), *beamsPerSite,
		getRockTypeName(), getClimateName())
}

func getRockTypeName() string {
	if currentRock != nil {
		return currentRock.RockType
	}
	return "mixed"
}

func getClimateName() string {
	if currentClimate != nil {
		return currentClimate.ClimateType
	}
	return "mixed"
}

func generateReading(site SiteConfig, beamID int, t time.Time, hoursElapsed float64) SensorReading {
	dayFrac := float64(t.Hour())/24.0 + float64(t.Minute())/1440.0
	seasonal := math.Sin(2 * math.Pi * (float64(t.YearDay()) / 365.25))

	rockFactor := 1.0
	if site.RockConfig != nil {
		rockFactor = site.RockConfig.StrainInfluence
	}

	temp := site.BaseTemp + site.TempAmp*(-math.Cos(2*math.Pi*dayFrac))
	temp += seasonal * site.TempAmp * 0.5
	temp += rand.NormFloat64() * 0.8

	humid := site.HumidityBase - seasonal*10.0 + math.Sin(2*math.Pi*dayFrac)*8.0
	humid += rand.NormFloat64() * 3.0
	humid = math.Max(20, math.Min(98, humid))

	rainfall := 0.0
	if rand.Float64() < 0.15 {
		rainfall = math.Abs(rand.ExpFloat64() * 2.0 * site.RainFactor)
	}

	ageFactor := 1.0 + hoursElapsed*0.0001*site.DegradeFactor
	tempStress := math.Abs(temp - 15.0) * 8.0
	rainStress := rainfall * 5.0
	seasonalStress := (1.0 + seasonal) * 100.0

	strainBase := site.StrainBase * ageFactor * rockFactor
	beamVar := 1.0 + float64(beamID%5)*0.02

	beamStrainTop := strainBase*beamVar + tempStress + rainStress + seasonalStress*0.3 + rand.NormFloat64()*40.0
	beamStrainBottom := strainBase*beamVar*1.15 + tempStress*0.9 + rainStress*1.1 + rand.NormFloat64()*45.0
	beamStrainSide := strainBase*beamVar*0.85 + tempStress*0.7 + seasonalStress*0.2 + rand.NormFloat64()*35.0

	if shouldTriggerStrainAlarm(site, beamID, t) {
		factor := 2.0 + rand.Float64()*1.5
		beamStrainTop *= factor
		beamStrainBottom *= factor
		beamStrainSide *= factor
		log.Printf("⚠️  [应变告警] %s 梁#%d 岩性:%s", site.SiteName, beamID, site.RockType)
	}

	crackBase := site.CrackBase * (1.0 + hoursElapsed*0.0005*site.DegradeFactor)
	freezeThawFactor := 1.0
	if site.RockConfig != nil {
		freezeThawFactor = site.RockConfig.FreezeThawFactor
	}
	if temp < 2.0 && temp > -5.0 {
		freezeThawFactor *= 1.5
	}

	crackGrowthFactor := 1.0
	if site.RockConfig != nil {
		crackGrowthFactor = site.RockConfig.CrackGrowthRate / 1.0e-12
	}

	rockCrack1 := crackBase*freezeThawFactor*crackGrowthFactor + hoursElapsed*0.0003 + rand.NormFloat64()*crackBase*0.1
	rockCrack2 := crackBase*0.85*freezeThawFactor*crackGrowthFactor + hoursElapsed*0.00025 + rand.NormFloat64()*crackBase*0.08
	rockCrack3 := crackBase*1.1*freezeThawFactor*crackGrowthFactor + hoursElapsed*0.00035 + rand.NormFloat64()*crackBase*0.12

	rockCrack1 = math.Max(0.01, rockCrack1)
	rockCrack2 = math.Max(0.01, rockCrack2)
	rockCrack3 = math.Max(0.01, rockCrack3)

	if shouldTriggerCrackAlarm(site, beamID, t) {
		rockCrack1 *= 3.0
		rockCrack2 *= 2.8
		rockCrack3 *= 3.2
		log.Printf("⚠️  [裂隙告警] %s 梁#%d 岩性:%s 冻融系数:%.1f",
			site.SiteName, beamID, site.RockType, freezeThawFactor)
	}

	rockTypeTag := ""
	if site.RockConfig != nil {
		rockTypeTag = site.RockConfig.RockType
	}
	climateTypeTag := ""
	if site.ClimateConfig != nil {
		climateTypeTag = site.ClimateConfig.ClimateType
	}

	return SensorReading{
		Time:             t.UTC().Format(time.RFC3339Nano),
		SiteID:           site.SiteID,
		BeamID:           beamID,
		BeamStrainTop:    round6(beamStrainTop),
		BeamStrainBottom: round6(beamStrainBottom),
		BeamStrainSide:   round6(beamStrainSide),
		RockCrackWidth1:  round6(rockCrack1),
		RockCrackWidth2:  round6(rockCrack2),
		RockCrackWidth3:  round6(rockCrack3),
		Temperature:      round2(temp),
		Humidity:         round2(humid),
		Rainfall:         round2(rainfall),
		RockType:         rockTypeTag,
		ClimateType:      climateTypeTag,
	}
}

func shouldTriggerStrainAlarm(site SiteConfig, beamID int, t time.Time) bool {
	baseChance := *anomalyChance * 0.4
	if t.Hour() >= 11 && t.Hour() <= 15 {
		if site.SiteID == beamID%10+1 {
			return rand.Float64() < baseChance*1.6
		}
	}
	return rand.Float64() < baseChance
}

func shouldTriggerCrackAlarm(site SiteConfig, beamID int, t time.Time) bool {
	baseChance := *anomalyChance * 0.3
	dailyPeak := (t.Hour() >= 2 && t.Hour() <= 6) || (t.Hour() >= 14 && t.Hour() <= 18)
	if dailyPeak {
		if site.DegradeFactor > 1.3 && (site.SiteID+beamID)%7 == 0 {
			return rand.Float64() < baseChance*2.0
		}
	}
	return rand.Float64() < baseChance
}

func sendBatch(batch []SensorReading) {
	if *useMQTT && mqttClient != nil && mqttClient.IsConnected() {
		for _, r := range batch {
			payload, _ := json.Marshal(r)
			topic := fmt.Sprintf("%s%d", *mqttTopic, r.SiteID)
			token := mqttClient.Publish(topic, 1, false, payload)
			token.WaitTimeout(100 * time.Millisecond)
		}
		return
	}

	payload, err := json.Marshal(batch)
	if err != nil {
		log.Printf("JSON encode error: %v", err)
		return
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("POST", *apiURL, bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("HTTP request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("发送失败: %v", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		log.Printf("API警告: HTTP %d", resp.StatusCode)
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
