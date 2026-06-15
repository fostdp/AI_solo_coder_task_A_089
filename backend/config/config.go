package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	MQTT     MQTTConfig
	FEM      FEMConfig
	Weather  WeatherConfig
}

type ServerConfig struct {
	Port int
	Mode string
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
	PoolMax  int
}

type MQTTConfig struct {
	Broker      string
	ClientID    string
	Username    string
	Password    string
	AlarmTopic  string
	DataTopic   string
}

type FEMConfig struct {
	ElementSize   float64
	MaxIterations int
	Tolerance     float64
}

type WeatherConfig struct {
	FreezeTemp        float64
	ThawTemp          float64
	CriticalCrackDepth float64
	WoodDesignLife    float64
	RockDesignLife    float64
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	return &Config{
		Server: ServerConfig{
			Port: getEnvInt("SERVER_PORT", 8080),
			Mode: getEnvStr("SERVER_MODE", "debug"),
		},
		Database: DatabaseConfig{
			Host:     getEnvStr("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnvStr("DB_USER", "postgres"),
			Password: getEnvStr("DB_PASSWORD", "postgres"),
			Name:     getEnvStr("DB_NAME", "plankroad_monitor"),
			SSLMode:  getEnvStr("DB_SSLMODE", "disable"),
			PoolMax:  getEnvInt("DB_POOL_MAX", 25),
		},
		MQTT: MQTTConfig{
			Broker:     getEnvStr("MQTT_BROKER", "tcp://localhost:1883"),
			ClientID:   getEnvStr("MQTT_CLIENT_ID", "plankroad_backend"),
			Username:   getEnvStr("MQTT_USERNAME", "admin"),
			Password:   getEnvStr("MQTT_PASSWORD", "admin"),
			AlarmTopic: getEnvStr("MQTT_ALARM_TOPIC", "plankroad/alarm/+"),
			DataTopic:  getEnvStr("MQTT_DATA_TOPIC", "plankroad/data/+"),
		},
		FEM: FEMConfig{
			ElementSize:   getEnvFloat("FEM_ELEMENT_SIZE", 0.1),
			MaxIterations: getEnvInt("FEM_MAX_ITERATIONS", 1000),
			Tolerance:     getEnvFloat("FEM_TOLERANCE", 1e-6),
		},
		Weather: WeatherConfig{
			FreezeTemp:         getEnvFloat("FREEZE_TEMP_THRESHOLD", 0.0),
			ThawTemp:           getEnvFloat("THAW_TEMP_THRESHOLD", 2.0),
			CriticalCrackDepth: getEnvFloat("CRITICAL_CRACK_DEPTH", 50.0),
			WoodDesignLife:     getEnvFloat("WOOD_DESIGN_LIFE", 200.0),
			RockDesignLife:     getEnvFloat("ROCK_DESIGN_LIFE", 1000.0),
		},
	}
}

func getEnvStr(key, defaultValue string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if v, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultValue
}
