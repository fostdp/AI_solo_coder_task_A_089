package params

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type PoreStructure struct {
	Porosity      float64 `json:"孔隙率"`
	AvgPoreRadius float64 `json:"平均孔径_nm"`
	PoreSizeDisp  float64 `json:"孔径分散度"`
	Saturation    float64 `json:"饱水度"`
	Permeability  float64 `json:"渗透率_m2"`
}

type ThermalProps struct {
	AlphaParallel float64 `json:"平行层理热膨胀系数_/K"`
	AlphaNormal   float64 `json:"垂直层理热膨胀系数_/K"`
	Conductivity  float64 `json:"导热系数_W/mK"`
	HeatCapacity  float64 `json:"比热容_J/kgK"`
	AnisotropyIdx float64 `json:"热各向异性指数"`
}

type RockParams struct {
	RockClass       string        `json:"岩石分类"`
	HardnessMohs    float64       `json:"莫氏硬度"`
	K_IC            float64       `json:"断裂韧性_MPa√m"`
	C               float64       `json:"Paris公式_C"`
	M               float64       `json:"Paris公式_m"`
	Pore            PoreStructure `json:"孔隙结构"`
	Thermal         ThermalProps  `json:"热学性质"`
	FreezePressure  float64       `json:"冻胀压力_MPa"`
	FTDamageBeta    float64       `json:"冻融损伤指数_β"`
	ChemicalFactor  float64       `json:"化学溶蚀因子"`
	ShockFactor     float64       `json:"热冲击系数"`
	BeddingFactor   float64       `json:"层理加速因子"`
}

type RockCommonConst struct {
	GammaSL      float64 `json:"固液界面张力_N/m"`
	RhoIce       float64 `json:"冰的密度_kg/m3"`
	LatentHeat   float64 `json:"水的熔化潜热_J/kg"`
	T0           float64 `json:"冰点_K"`
	VolExpansion float64 `json:"水结冰体积膨胀率"`
}

type RockParamsRoot struct {
	RockParamConfig struct {
		Note          string `json:"说明"`
		Version       string `json:"版本"`
		UpdateDate    string `json:"更新日期"`
		CommonConst   RockCommonConst `json:"通用物理常数"`
		RockTypes map[string]RockParams `json:"岩石类型"`
	} `json:"岩石参数配置"`
}

type WoodParams struct {
	Family          string  `json:"中文科属"`
	InitMoisture    float64 `json:"初始含水率_%"`
	DecayCoeff      float64 `json:"腐朽系数_年-1"`
	TempCoeff       float64 `json:"温度系数"`
	HumidCoeff      float64 `json:"湿度系数"`
	BiologicalFactor float64 `json:"生物因子"`
	LigninContent   float64 `json:"木质素含量"`
	DurabilityClass int     `json:"耐久等级"`
	AirDryDensity   float64 `json:"气干密度_kg/m3"`
	BendingStrength float64 `json:"抗弯强度_MPa"`
	CompressionStrength float64 `json:"顺纹抗压强度_MPa"`
	ElasticModulus  float64 `json:"弹性模量_GPa"`
}

type WoodCommonConst struct {
	PerpendicularCompRatio  float64 `json:"横纹抗压强度比"`
	RadialShrinkCoeff      float64 `json:"湿胀干缩系数_径向"`
	TangentialShrinkCoeff  float64 `json:"湿胀干缩系数_弦向"`
	FiberSaturationPoint   float64 `json:"纤维饱和点_%"`
	OvenDryDensityBaseline float64 `json:"绝干密度_kg/m3_基准"`
}

type WoodParamsRoot struct {
	WoodParamConfig struct {
		Note       string `json:"说明"`
		Version    string `json:"版本"`
		UpdateDate string `json:"更新日期"`
		CommonConst WoodCommonConst `json:"通用木材常数"`
		WoodTypes map[string]WoodParams `json:"木材类型"`
	} `json:"木材参数配置"`
}

var (
	LoadedRockParams map[string]RockParams
	LoadedWoodParams map[string]WoodParams
	RockCommon  *RockCommonConst
	WoodCommon *WoodCommonConst
)

func LoadRockParams(configDir string) error {
	path := filepath.Join(configDir, "rock_params.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read rock_params.json: %w", err)
	}

	var root RockParamsRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse rock_params.json: %w", err)
	}

	LoadedRockParams = make(map[string]RockParams)
	for name, p := range root.RockParamConfig.RockTypes {
		LoadedRockParams[name] = p
	}
	RockCommon = &root.RockParamConfig.CommonConst
	return nil
}

func LoadWoodParams(configDir string) error {
	path := filepath.Join(configDir, "wood_params.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read wood_params.json: %w", err)
	}

	var root WoodParamsRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse wood_params.json: %w", err)
	}

	LoadedWoodParams = make(map[string]WoodParams)
	for name, p := range root.WoodParamConfig.WoodTypes {
		LoadedWoodParams[name] = p
	}
	WoodCommon = &root.WoodParamConfig.CommonConst
	return nil
}

func LoadAll(configDir string) error {
	if err := LoadRockParams(configDir); err != nil {
		return err
	}
	if err := LoadWoodParams(configDir); err != nil {
		return err
	}
	return nil
}

func GetRockParams(name string) (RockParams, bool) {
	p, ok := LoadedRockParams[name]
	return p, ok
}

func GetWoodParams(name string) (WoodParams, bool) {
	p, ok := LoadedWoodParams[name]
	return p, ok
}
