-- ============================================================
-- 古代栈道结构力学仿真与风化评估系统 - TimescaleDB初始化脚本
-- Ancient Plank Road Structural Mechanics & Weathering Assessment
-- ============================================================

-- 创建数据库
CREATE DATABASE plankroad_monitor;
\c plankroad_monitor;

-- 启用TimescaleDB扩展
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS postgis;

-- ============================================================
-- 1. 栈道遗址表 (栈道基础信息)
-- ============================================================
CREATE TABLE IF NOT EXISTS plankroad_sites (
    site_id         SERIAL PRIMARY KEY,
    site_name       VARCHAR(100) NOT NULL,
    location        GEOGRAPHY(POINT, 4326),
    region          VARCHAR(50),
    elevation       NUMERIC(8,2),
    construction_era VARCHAR(50),
    total_length    NUMERIC(10,2),
    beam_count      INTEGER,
    rock_type       VARCHAR(50),
    wood_type       VARCHAR(50),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- 插入10处秦巴山区栈道遗址
INSERT INTO plankroad_sites (site_name, location, region, elevation, construction_era, total_length, beam_count, rock_type, wood_type) VALUES
('明月峡古栈道', ST_SetSRID(ST_MakePoint(105.82, 32.44), 4326)::GEOGRAPHY, '四川广元', 485.5, '战国-明清', 2000.0, 486, '石灰岩', '柏木'),
('石门栈道',    ST_SetSRID(ST_MakePoint(107.03, 33.22), 4326)::GEOGRAPHY, '陕西汉中', 620.0, '秦汉-明清', 1500.0, 365, '花岗岩', '青冈木'),
('子午道遗址',   ST_SetSRID(ST_MakePoint(108.78, 33.85), 4326)::GEOGRAPHY, '陕西安康', 890.0, '秦汉', 3200.0, 782, '片麻岩', '松木'),
('褒斜道遗址',   ST_SetSRID(ST_MakePoint(107.82, 33.95), 4326)::GEOGRAPHY, '陕西宝鸡', 750.0, '战国-唐宋', 4500.0, 1098, '石灰岩', '栎木'),
('陈仓道遗址',   ST_SetSRID(ST_MakePoint(106.85, 34.28), 4326)::GEOGRAPHY, '陕西宝鸡', 920.0, '秦汉', 2800.0, 695, '大理岩', '杉木'),
('金牛道遗址',   ST_SetSRID(ST_MakePoint(105.76, 32.15), 4326)::GEOGRAPHY, '四川绵阳', 540.0, '战国-明清', 3800.0, 925, '砂岩', '柏木'),
('米仓道遗址',   ST_SetSRID(ST_MakePoint(106.68, 32.58), 4326)::GEOGRAPHY, '四川巴中', 1100.0, '秦汉', 2200.0, 538, '板岩', '青冈木'),
('傥骆道遗址',   ST_SetSRID(ST_MakePoint(107.95, 33.62), 4326)::GEOGRAPHY, '陕西汉中', 1350.0, '唐代', 1800.0, 442, '片麻岩', '松木'),
('荔枝道遗址',   ST_SetSRID(ST_MakePoint(108.35, 32.08), 4326)::GEOGRAPHY, '四川达州', 680.0, '唐代', 1200.0, 298, '砂岩', '杉木'),
('阴平道遗址',   ST_SetSRID(ST_MakePoint(104.92, 32.85), 4326)::GEOGRAPHY, '四川广元', 1450.0, '三国', 900.0, 225, '石灰岩', '栎木');

-- ============================================================
-- 2. 传感器数据表 (时序数据 - hypertable)
-- 每处栈道每1小时上报：梁孔应变、岩体裂隙宽度、温湿度
-- ============================================================
CREATE TABLE IF NOT EXISTS sensor_readings (
    time            TIMESTAMPTZ NOT NULL,
    site_id         INTEGER NOT NULL REFERENCES plankroad_sites(site_id),
    beam_id         INTEGER NOT NULL,
    -- 梁孔应变 (με 微应变)
    beam_strain_top    NUMERIC(12,6),
    beam_strain_bottom NUMERIC(12,6),
    beam_strain_side   NUMERIC(12,6),
    -- 岩体裂隙宽度 (mm)
    rock_crack_width_1 NUMERIC(10,6),
    rock_crack_width_2 NUMERIC(10,6),
    rock_crack_width_3 NUMERIC(10,6),
    -- 环境数据
    temperature     NUMERIC(6,2),
    humidity        NUMERIC(5,2),
    rainfall        NUMERIC(8,2),
    -- 计算辅助字段
    avg_strain      NUMERIC(12,6),
    max_crack_width NUMERIC(10,6),
    -- 告警标志
    strain_alarm    BOOLEAN DEFAULT FALSE,
    crack_alarm     BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (time, site_id, beam_id)
);

-- 转换为TimescaleDB hypertable，按时间分区（1天一个chunk）
SELECT create_hypertable('sensor_readings', 'time', 
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

-- 创建复合索引加速查询
CREATE INDEX IF NOT EXISTS idx_sensor_site_time ON sensor_readings(site_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_sensor_beam ON sensor_readings(site_id, beam_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_sensor_alarm ON sensor_readings(strain_alarm, crack_alarm, time DESC);

-- ============================================================
-- 3. 结构仿真结果表 (有限元计算结果)
-- ============================================================
CREATE TABLE IF NOT EXISTS structural_simulation (
    sim_id          SERIAL PRIMARY KEY,
    site_id         INTEGER NOT NULL REFERENCES plankroad_sites(site_id),
    sim_time        TIMESTAMPTZ NOT NULL,
    -- 材料参数
    wood_elastic_modulus NUMERIC(12,2),
    rock_elastic_modulus NUMERIC(12,2),
    wood_poisson_ratio   NUMERIC(5,4),
    rock_poisson_ratio   NUMERIC(5,4),
    -- 载荷
    dead_load       NUMERIC(10,4),
    live_load       NUMERIC(10,4),
    thermal_load    NUMERIC(10,4),
    -- 计算结果 (应力单位: MPa)
    max_wood_stress     NUMERIC(12,6),
    min_wood_stress     NUMERIC(12,6),
    max_rock_stress     NUMERIC(12,6),
    min_rock_stress     NUMERIC(12,6),
    max_deflection_mm   NUMERIC(10,6),
    safety_factor       NUMERIC(8,4),
    -- 节点数据 JSON (存储有限元网格和应力分布)
    element_data    JSONB,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sim_site_time ON structural_simulation(site_id, sim_time DESC);

-- ============================================================
-- 4. 风化评估结果表
-- ============================================================
CREATE TABLE IF NOT EXISTS weathering_assessment (
    assess_id       SERIAL PRIMARY KEY,
    site_id         INTEGER NOT NULL REFERENCES plankroad_sites(site_id),
    assess_time     TIMESTAMPTZ NOT NULL,
    -- 冻融循环参数
    freeze_thaw_cycles   INTEGER,
    current_crack_depth  NUMERIC(10,4),
    crack_propagation_rate NUMERIC(10,6),
    -- 风化评估
    weathering_grade     VARCHAR(20),
    wood_decay_rate      NUMERIC(10,6),
    rock_erosion_rate    NUMERIC(10,6),
    -- 预测寿命 (年)
    predicted_lifespan   NUMERIC(10,2),
    remaining_lifespan   NUMERIC(10,2),
    -- 置信度
    confidence           NUMERIC(5,4),
    -- 详细数据
    detail_data          JSONB,
    created_at           TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_weather_site_time ON weathering_assessment(site_id, assess_time DESC);

-- ============================================================
-- 5. 告警事件表
-- ============================================================
CREATE TABLE IF NOT EXISTS alarm_events (
    alarm_id        BIGSERIAL PRIMARY KEY,
    time            TIMESTAMPTZ NOT NULL,
    site_id         INTEGER NOT NULL REFERENCES plankroad_sites(site_id),
    beam_id         INTEGER,
    alarm_type      VARCHAR(30) NOT NULL,
    alarm_level     VARCHAR(20) NOT NULL,
    -- 告警详情
    metric_name     VARCHAR(50),
    current_value   NUMERIC(12,6),
    threshold_value NUMERIC(12,6),
    description     TEXT,
    -- 处理状态
    acknowledged    BOOLEAN DEFAULT FALSE,
    ack_time        TIMESTAMPTZ,
    ack_user        VARCHAR(50),
    resolved        BOOLEAN DEFAULT FALSE,
    resolve_time    TIMESTAMPTZ,
    mqtt_published  BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('alarm_events', 'time', 
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_alarm_site ON alarm_events(site_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_alarm_unresolved ON alarm_events(resolved, alarm_level, time DESC);

-- ============================================================
-- 6. 告警阈值配置表
-- ============================================================
CREATE TABLE IF NOT EXISTS alarm_thresholds (
    threshold_id    SERIAL PRIMARY KEY,
    site_id         INTEGER REFERENCES plankroad_sites(site_id),
    -- 梁孔应变阈值 (με)
    strain_warning  NUMERIC(12,2) DEFAULT 800.0,
    strain_critical NUMERIC(12,2) DEFAULT 1200.0,
    -- 裂隙宽度阈值 (mm)
    crack_warning   NUMERIC(10,4) DEFAULT 3.0,
    crack_critical  NUMERIC(10,4) DEFAULT 5.0,
    -- 裂隙扩展速率阈值 (mm/月)
    crack_rate_warning  NUMERIC(10,6) DEFAULT 0.1,
    crack_rate_critical NUMERIC(10,6) DEFAULT 0.3,
    -- 安全系数阈值
    sf_warning      NUMERIC(8,4) DEFAULT 1.5,
    sf_critical     NUMERIC(8,4) DEFAULT 1.2,
    is_active       BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- 为每个遗址设置默认阈值
INSERT INTO alarm_thresholds (site_id) SELECT generate_series(1, 10);

-- ============================================================
-- 7. 连续聚合视图 - 每日汇总
-- ============================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS sensor_daily_summary
WITH (timescaledb.continuous) AS
SELECT
    site_id,
    time_bucket('1 day', time) AS bucket_day,
    COUNT(*) AS reading_count,
    AVG(avg_strain) AS avg_daily_strain,
    MAX(avg_strain) AS max_daily_strain,
    AVG(max_crack_width) AS avg_daily_crack,
    MAX(max_crack_width) AS max_daily_crack,
    AVG(temperature) AS avg_temp,
    MIN(temperature) AS min_temp,
    MAX(temperature) AS max_temp,
    AVG(humidity) AS avg_humidity,
    SUM(CASE WHEN strain_alarm OR crack_alarm THEN 1 ELSE 0 END) AS alarm_count
FROM sensor_readings
GROUP BY site_id, time_bucket('1 day', time)
WITH NO DATA;

-- 配置刷新策略 (每小时刷新)
SELECT add_continuous_aggregate_policy('sensor_daily_summary',
    start_offset => INTERVAL '3 days',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);

-- ============================================================
-- 8. 实用函数
-- ============================================================

-- 计算两点间的应变差增长率
CREATE OR REPLACE FUNCTION calc_strain_rate(
    curr_strain NUMERIC, 
    prev_strain NUMERIC, 
    hours_diff NUMERIC
) RETURNS NUMERIC AS $$
BEGIN
    IF hours_diff <= 0 OR prev_strain = 0 THEN
        RETURN 0;
    END IF;
    RETURN ((curr_strain - prev_strain) / prev_strain) * (24.0 / hours_diff) * 100.0;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- 裂隙扩展速率计算 (Paris公式简化版: da/dN = C*(ΔK)^m)
CREATE OR REPLACE FUNCTION calc_crack_growth_rate(
    stress_range NUMERIC,    -- 应力范围 MPa
    crack_width NUMERIC,     -- 当前裂隙宽度 mm
    rock_toughness NUMERIC DEFAULT 1.5  -- 断裂韧性 MPa·m^0.5
) RETURNS NUMERIC AS $$
DECLARE
    C CONSTANT NUMERIC := 1.0e-12;  -- 材料常数
    m CONSTANT NUMERIC := 3.0;       -- Paris指数
    pi CONSTANT NUMERIC := 3.14159265358979;
    delta_k NUMERIC;
    growth_rate NUMERIC;
BEGIN
    delta_k := stress_range * SQRT(pi * crack_width / 1000.0);
    IF delta_k >= rock_toughness THEN
        RETURN 999.0;  -- 超过断裂韧性, 急速扩展
    END IF;
    growth_rate := C * POWER(delta_k, m) * 1000.0;
    RETURN ROUND(growth_rate, 9);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ============================================================
-- 9. 触发器 - 数据插入时自动计算派生字段和检测告警
-- ============================================================
CREATE OR REPLACE FUNCTION process_sensor_reading()
RETURNS TRIGGER AS $$
DECLARE
    thresh RECORD;
BEGIN
    -- 计算平均应变
    NEW.avg_strain := (COALESCE(NEW.beam_strain_top,0) + COALESCE(NEW.beam_strain_bottom,0) + COALESCE(NEW.beam_strain_side,0)) / 3.0;
    -- 计算最大裂隙宽度
    NEW.max_crack_width := GREATEST(
        COALESCE(NEW.rock_crack_width_1,0),
        COALESCE(NEW.rock_crack_width_2,0),
        COALESCE(NEW.rock_crack_width_3,0)
    );
    
    -- 获取阈值配置
    SELECT * INTO thresh FROM alarm_thresholds 
    WHERE (site_id = NEW.site_id OR site_id IS NULL) AND is_active = TRUE
    ORDER BY site_id NULLS LAST LIMIT 1;
    
    IF FOUND THEN
        NEW.strain_alarm := (NEW.avg_strain >= thresh.strain_warning);
        NEW.crack_alarm  := (NEW.max_crack_width >= thresh.crack_warning);
        
        -- 生成告警记录
        IF NEW.avg_strain >= thresh.strain_critical THEN
            INSERT INTO alarm_events (time, site_id, beam_id, alarm_type, alarm_level,
                metric_name, current_value, threshold_value, description)
            VALUES (NEW.time, NEW.site_id, NEW.beam_id, 'STRAIN', 'CRITICAL',
                'beam_strain', NEW.avg_strain, thresh.strain_critical,
                FORMAT('梁孔应变严重超限: %.2f με, 阈值: %.2f με', NEW.avg_strain, thresh.strain_critical));
        ELSIF NEW.avg_strain >= thresh.strain_warning THEN
            INSERT INTO alarm_events (time, site_id, beam_id, alarm_type, alarm_level,
                metric_name, current_value, threshold_value, description)
            VALUES (NEW.time, NEW.site_id, NEW.beam_id, 'STRAIN', 'WARNING',
                'beam_strain', NEW.avg_strain, thresh.strain_warning,
                FORMAT('梁孔应变告警: %.2f με, 阈值: %.2f με', NEW.avg_strain, thresh.strain_warning));
        END IF;
        
        IF NEW.max_crack_width >= thresh.crack_critical THEN
            INSERT INTO alarm_events (time, site_id, beam_id, alarm_type, alarm_level,
                metric_name, current_value, threshold_value, description)
            VALUES (NEW.time, NEW.site_id, NEW.beam_id, 'CRACK', 'CRITICAL',
                'rock_crack_width', NEW.max_crack_width, thresh.crack_critical,
                FORMAT('岩体裂隙严重超限: %.4f mm, 阈值: %.4f mm', NEW.max_crack_width, thresh.crack_critical));
        ELSIF NEW.max_crack_width >= thresh.crack_warning THEN
            INSERT INTO alarm_events (time, site_id, beam_id, alarm_type, alarm_level,
                metric_name, current_value, threshold_value, description)
            VALUES (NEW.time, NEW.site_id, NEW.beam_id, 'CRACK', 'WARNING',
                'rock_crack_width', NEW.max_crack_width, thresh.crack_warning,
                FORMAT('岩体裂隙告警: %.4f mm, 阈值: %.4f mm', NEW.max_crack_width, thresh.crack_warning));
        END IF;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_process_reading ON sensor_readings;
CREATE TRIGGER trg_process_reading
BEFORE INSERT ON sensor_readings
FOR EACH ROW EXECUTE FUNCTION process_sensor_reading();

-- ============================================================
-- 完成
-- ============================================================
\echo '========================================'
\echo '古代栈道监测数据库初始化完成!'
\echo 'TimescaleDB hypertables 已创建'
\echo '10处秦巴山区栈道遗址已录入'
\echo '告警触发器已启用'
\echo '========================================'
