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
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type SiteConfig struct {
	SiteID    int
	SiteName  string
	Elevation float64
	BeamCount int
	BaseTemp  float64
	TempAmp   float64
	HumidityBase float64
	StrainBase float64
	CrackBase float64
	RockType  string
	RainFactor float64
	DegradeFactor float64
}

var Sites = []SiteConfig{
	{1, "明月峡古栈道", 485.5, 486, 16.5, 12.0, 75.0, 350.0, 1.2, "石灰岩", 1.3, 1.1},
	{2, "石门栈道",    620.0, 365, 14.5, 14.0, 70.0, 420.0, 0.9, "花岗岩", 1.1, 0.9},
	{3, "子午道遗址",   890.0, 782, 12.0, 16.0, 65.0, 280.0, 1.8, "片麻岩", 0.9, 1.3},
	{4, "褒斜道遗址",   750.0, 1098, 13.0, 15.0, 68.0, 310.0, 1.5, "石灰岩", 1.0, 1.2},
	{5, "陈仓道遗址",   920.0, 695, 11.5, 17.0, 62.0, 380.0, 2.1, "大理岩", 0.8, 1.4},
	{6, "金牛道遗址",   540.0, 925, 17.0, 10.0, 78.0, 340.0, 1.6, "砂岩", 1.4, 1.5},
	{7, "米仓道遗址",   1100.0, 538, 9.5, 18.0, 72.0, 260.0, 2.5, "板岩", 1.2, 1.6},
	{8, "傥骆道遗址",   1350.0, 442, 7.0, 20.0, 60.0, 450.0, 3.0, "片麻岩", 0.7, 1.7},
	{9, "荔枝道遗址",   680.0, 298, 15.5, 13.0, 76.0, 290.0, 1.7, "砂岩", 1.3, 1.3},
	{10, "阴平道遗址",  1450.0, 225, 6.0, 22.0, 58.0, 520.0, 3.5, "石灰岩", 0.6, 1.8},
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
}

var (
	apiURL    = flag.String("api", "http://localhost:8080/api/sensor/batch", "Backend API endpoint")
	mqttBroker = flag.String("mqtt", "tcp://localhost:1883", "MQTT broker address")
	interval  = flag.Duration("interval", 1*time.Hour, "Reporting interval (use shorter for testing)")
	beamsPerSite = flag.Int("beams", 5, "Beams per site to simulate")
	backfill  = flag.Int("backfill", 0, "Hours of historical data to generate on start")
	useMQTT   = flag.Bool("mqtt-enable", false, "Publish via MQTT instead of HTTP")
	mqttClient mqtt.Client
	startHour = float64(0)
)

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	log.Println("===========================================")
	log.Println("古代栈道传感器模拟器启动")
	log.Printf("遗址数量: %d", len(Sites))
	log.Printf("每遗址梁数: %d", *beamsPerSite)
	log.Printf("上报间隔: %v", *interval)
	log.Printf("后端API: %s", *apiURL)
	log.Printf("MQTT启用: %v", *useMQTT)
	if *useMQTT {
		log.Printf("MQTT Broker: %s", *mqttBroker)
		if err := setupMQTT(); err != nil {
			log.Printf("MQTT setup warning: %v", err)
		}
	}
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

func setupMQTT() error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(*mqttBroker)
	opts.SetClientID("plankroad_simulator")
	opts.SetAutoReconnect(true)

	mqttClient = mqtt.NewClient(opts)
	token := mqttClient.Connect()
	token.WaitTimeout(10 * time.Second)
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
		batch := make([]SensorReading, 0, step*len(Sites)*(*beamsPerSite))

		for hi := 0; hi < step; hi++ {
			t := time.Now().Add(-time.Duration(h-hi) * time.Hour)
			for _, site := range Sites {
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
	batch := make([]SensorReading, 0, len(Sites)*(*beamsPerSite))
	startHour += (*interval).Hours()

	for _, site := range Sites {
		for beam := 1; beam <= *beamsPerSite; beam++ {
			batch = append(batch, generateReading(site, beam, t, startHour))
		}
	}

	sendBatch(batch)
	log.Printf("[%s] 上报完成: %d 条数据 (10遗址x%d梁)",
		t.Format("2006-01-02 15:04"), len(batch), *beamsPerSite)
}

func generateReading(site SiteConfig, beamID int, t time.Time, hoursElapsed float64) SensorReading {
	dayFrac := float64(t.Hour())/24.0 + float64(t.Minute())/1440.0
	seasonal := math.Sin(2 * math.Pi * (float64(t.YearDay()) / 365.25))

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

	strainBase := site.StrainBase * ageFactor
	beamVar := 1.0 + float64(beamID%5)*0.02

	beamStrainTop := strainBase*beamVar + tempStress + rainStress + seasonalStress*0.3 + rand.NormFloat64()*40.0
	beamStrainBottom := strainBase*beamVar*1.15 + tempStress*0.9 + rainStress*1.1 + rand.NormFloat64()*45.0
	beamStrainSide := strainBase*beamVar*0.85 + tempStress*0.7 + seasonalStress*0.2 + rand.NormFloat64()*35.0

	if shouldTriggerStrainAlarm(site, beamID, t) {
		factor := 2.0 + rand.Float64()*1.5
		beamStrainTop *= factor
		beamStrainBottom *= factor
		beamStrainSide *= factor
		log.Printf("⚠️  [告警触发] %s 梁#%d 梁孔应变异常!", site.SiteName, beamID)
	}

	crackBase := site.CrackBase * (1.0 + hoursElapsed*0.0005*site.DegradeFactor)
	freezeThawFactor := 1.0
	if temp < 2.0 && temp > -5.0 {
		freezeThawFactor = 1.5
	}

	rockCrack1 := crackBase*freezeThawFactor + hoursElapsed*0.0003 + rand.NormFloat64()*crackBase*0.1
	rockCrack2 := crackBase*0.85*freezeThawFactor + hoursElapsed*0.00025 + rand.NormFloat64()*crackBase*0.08
	rockCrack3 := crackBase*1.1*freezeThawFactor + hoursElapsed*0.00035 + rand.NormFloat64()*crackBase*0.12

	rockCrack1 = math.Max(0.01, rockCrack1)
	rockCrack2 = math.Max(0.01, rockCrack2)
	rockCrack3 = math.Max(0.01, rockCrack3)

	if shouldTriggerCrackAlarm(site, beamID, t) {
		rockCrack1 *= 3.0
		rockCrack2 *= 2.8
		rockCrack3 *= 3.2
		log.Printf("⚠️  [告警触发] %s 梁#%d 岩体裂隙扩展!", site.SiteName, beamID)
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
	}
}

func shouldTriggerStrainAlarm(site SiteConfig, beamID int, t time.Time) bool {
	if t.Hour() >= 11 && t.Hour() <= 15 {
		if site.SiteID == beamID%10+1 {
			return rand.Float64() < 0.08
		}
	}
	return rand.Float64() < 0.02
}

func shouldTriggerCrackAlarm(site SiteConfig, beamID int, t time.Time) bool {
	dailyPeak := (t.Hour() >= 2 && t.Hour() <= 6) || (t.Hour() >= 14 && t.Hour() <= 18)
	if dailyPeak {
		if site.DegradeFactor > 1.3 && (site.SiteID+beamID)%7 == 0 {
			return rand.Float64() < 0.1
		}
	}
	return rand.Float64() < 0.015
}

func sendBatch(batch []SensorReading) {
	if *useMQTT && mqttClient != nil && mqttClient.IsConnected() {
		for _, r := range batch {
			payload, _ := json.Marshal(r)
			topic := fmt.Sprintf("plankroad/data/%d", r.SiteID)
			mqttClient.Publish(topic, 1, false, payload)
		}
		time.Sleep(100 * time.Millisecond)
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
