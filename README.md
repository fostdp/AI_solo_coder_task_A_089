# 古代栈道结构力学仿真与风化评估系统

Ancient Plank Road Structural Mechanics & Weathering Assessment System

## 项目简介

本系统针对秦巴山区10处古代栈道遗址（明月峡、石门、子午道等）进行长期结构健康监测。通过模拟传感器每小时采集梁孔应变、岩体裂隙宽度、温湿度数据，基于有限元法(FEM)进行结构力学仿真，结合冻融循环模型预测岩体裂隙扩展和栈道保存寿命，并通过MQTT实现实时告警推送。

## 系统架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                                 前端浏览器                                    │
│  ┌──────────────────┐           ┌──────────────────┐                           │
│  │ plank_road_3d.js │           │ weathering_panel │  <- ES Modules            │
│  │  (Three.js 3D)  │           │   (数据面板)      │                           │
│  └─────────┬────────┘           └─────────┬────────┘                           │
│            │                              │                                    │
│            └──────────────┬───────────────┘                                    │
│                           │ app.js (协调器)                                     │
└───────────────────────────┼─────────────────────────────────────────────────────┘
                            │ REST API + WebSocket
┌───────────────────────────┼─────────────────────────────────────────────────────┐
│  ┌────────────────────────▼───────────────────────────────┐                     │
│  │                  Gin HTTP Server                       │                     │
│  │  ┌────────┐  ┌──────┐  ┌────────┐  ┌──────────────┐ │                     │
│  │  │  Gzip  │  │Metrics│  │  CORS  │  │    Logger    │ │  <- Middlewares      │
│  │  └────────┘  └──────┘  └────────┘  └──────────────┘ │                     │
│  └───────────────────────┬───────────────────────────────┘                     │
│                          │                                                       │
│  ┌───────────────────────────────────────────────────────────────────┐          │
│  │  Bus (Channel Pub/Sub + RPC)                                       │          │
│  └──┬──────────┬──────────┬──────────┬───────────────────────────────┘          │
│     │          │          │          │                                          │
│  ┌──▼──┐   ┌───▼───┐  ┌───▼─────┐  ┌──▼──────┐                                 │
│  │ DTU │   │Struct.│  │Weather. │  │ Alarm   │  <- 4个模块                    │
│  │Recv │   │Simula.│  │Evaluator│  │  MQTT   │                                 │
│  └──┬──┘   └───┬───┘  └───┬─────┘  └──┬──────┘                                 │
│     │          │          │             │                                         │
│  ┌──▼──────────▼──────────▼─────────────▼───────────────────────────┐             │
│  │                TimescaleDB (时序数据库)                           │             │
│  │  ┌────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐   │             │
│  │  │Raw Data│  │  Hourly  │  │  Daily   │  │ Continuous Agg.  │   │             │
│  │  │  3个月  │  │   1年    │  │   5年    │  │  压缩+降采样+保留 │   │             │
│  │  └────────┘  └──────────┘  └──────────┘  └──────────────────┘   │             │
│  └──────────────────────────────────────────────────────────────────┘             │
│                                                                                   │
│  ┌─────────────┐    ┌─────────────┐    ┌──────────────┐                         │
│  │   pprof     │    │  Prometheus │    │    MQTT      │  <- 监控                │
│  │  (:6060)    │    │  (:9090)    │    │  (:1883)     │                         │
│  └─────────────┘    └─────────────┘    └──────────────┘                         │
└───────────────────────────────────────────────────────────────────────────────────┘
                                              │
┌──────────────────────────────────────────────┼────────────────────────────────────┐
│                 传感器模拟器                    │ MQTT/HTTP                      │
│  ┌───────────────────────────────────────────▼──────────────────────────────┐    │
│  │  plankroad-simulator                                                    │    │
│  │  岩性: limestone/sandstone/granite/gneiss/shale/quartzite                │    │
│  │  气候: temperate/subtropical/alpine/arid                                 │    │
│  │  支持: 10处遗址、异常注入、历史数据回填、MQTT/HTTP双通道上报                │    │
│  └──────────────────────────────────────────────────────────────────────────┘    │
└───────────────────────────────────────────────────────────────────────────────────┘
```

## 技术栈

### 后端
- **语言**: Go 1.21+
- **Web框架**: Gin v1.9
- **时序数据库**: TimescaleDB 2.13 (PostgreSQL 16)
- **MQTT Broker**: Eclipse Mosquitto 2.0
- **数值计算**: Gonum (矩阵运算、有限元求解)
- **监控**: pprof + Prometheus
- **容器化**: Docker + Docker Compose

### 前端
- **3D渲染**: Three.js r160
- **UI**: 原生JavaScript + Canvas
- **模块**: ES Modules + Importmap
- **纹理**: PBR (BaseColor/Normal/Roughness/AO) 四通道渐进加载

### 核心算法
- **有限元法**: 四面体C3D4单元 + 接触算法(罚函数+Mohr-Coulomb+Newton-Raphson)
- **冻融风化**: 孔隙水9%体积膨胀 + Gibbs-Thomson冰点降低 + 热冲击开裂
- **裂隙扩展**: Paris公式 da/dN = C·(ΔK)^m
- **木材腐朽**: Arrhenius温度依赖 + 含水率-微生物活性模型

## 目录结构

```
.
├── backend/                    # Go后端服务
│   ├── config/                 # 配置
│   │   ├── config.go          # 主配置
│   │   └── params/            # 参数配置(JSON)
│   │       ├── rock_params.json    # 6种岩石参数
│   │       ├── wood_params.json    # 5种木材参数
│   │       ├── texture_params.json # PBR纹理参数
│   │       └── loader.go          # 参数加载器
│   ├── modules/                # 业务模块
│   │   ├── bus/                # 模块间通信总线
│   │   ├── dtu_receiver/       # 传感器数据采集校验
│   │   ├── structural_simulator/ # 有限元分析
│   │   ├── weathering_evaluator/ # 风化评估
│   │   └── alarm_mqtt/         # 告警推送
│   ├── middleware/             # Gin中间件
│   │   ├── gzip.go            # Gzip压缩
│   │   └── metrics.go         # Prometheus指标
│   ├── monitoring/             # 监控模块
│   │   └── metrics.go         # pprof + Prometheus
│   ├── simulation/             # 有限元求解器
│   ├── weathering/             # 风化评估算法
│   ├── repository/             # 数据访问层
│   ├── models/                 # 数据模型
│   ├── handlers/               # API处理器
│   ├── database/               # 数据库连接
│   ├── mqttclient/             # MQTT客户端
│   ├── static/                 # 前端静态资源
│   │   ├── index.html
│   │   ├── css/
│   │   └── js/
│   │       ├── app.js          # 主协调器
│   │       ├── plank_road_3d.js # 3D渲染模块
│   │       └── weathering_panel.js # 风化面板模块
│   └── main.go                 # 入口
├── simulator/                  # 传感器模拟器
│   └── simulator.go
├── db/                         # 数据库脚本
│   └── init.sql               # 初始化+降采样+保留策略
├── config/                     # 中间件配置
│   ├── mosquitto/             # MQTT配置
│   ├── prometheus/            # Prometheus配置
│   └── grafana/               # Grafana配置
├── Dockerfile                  # 多阶段构建
├── docker-compose.yml          # 服务编排
├── .env.example               # 环境变量示例
├── .dockerignore              # Docker忽略
└── README.md                  # 本文档
```

## 快速开始

### 前置要求
- Docker 24.0+
- Docker Compose v2.20+
- 至少4GB内存，10GB磁盘空间

### 一键部署

```bash
# 1. 复制环境变量配置
cp .env.example .env

# 2. 启动核心服务 (数据库 + MQTT + 后端)
docker-compose up -d timescale mqtt backend

# 3. 查看服务状态
docker-compose ps

# 4. 启动传感器模拟器 (可选)
docker-compose --profile simulator up -d simulator

# 5. 启动监控栈 (Prometheus + Grafana, 可选)
docker-compose --profile monitoring up -d
```

### 访问地址

| 服务 | 地址 | 说明 |
|------|------|------|
| 前端页面 | http://localhost:8080 | 栈道3D可视化系统 |
| API文档 | http://localhost:8080/api/sites | 遗址列表API |
| Metrics | http://localhost:9090/metrics | Prometheus指标 |
| PProf | http://localhost:6060/debug/pprof | 性能分析 |
| Prometheus | http://localhost:9091 | Prometheus UI (monitoring profile) |
| Grafana | http://localhost:3000 | 监控面板 (admin/admin2026) |
| MQTT | tcp://localhost:1883 | 消息队列 |

### 停止服务

```bash
# 停止所有服务
docker-compose down

# 停止并删除数据卷 (慎用)
docker-compose down -v

# 停止特定profile服务
docker-compose --profile simulator down
```

## 传感器模拟器使用

### 启动方式

**Docker方式 (推荐)**:
```bash
# 启动默认配置模拟器
docker-compose --profile simulator up -d simulator

# 自定义配置启动
docker run -d --name plankroad-simulator \
  -e MQTT_BROKER=tcp://mqtt:1883 \
  -e SIM_ROCK_TYPE=granite \
  -e SIM_CLIMATE=alpine \
  -e SIM_INTERVAL_SEC=60 \
  -e SIM_ANOMALY_CHANCE=0.1 \
  --network plankroad-network \
  plankroad-simulator
```

**本地编译运行**:
```bash
cd simulator
go build -o simulator .

# 基本用法
./simulator \
  -api http://localhost:8080 \
  -interval 1h \
  -beams 5 \
  -rock-type granite \
  -climate temperate \
  -anomaly 0.05 \
  -sites 1,2,3

# MQTT上报模式
./simulator \
  -mqtt-enable \
  -mqtt tcp://localhost:1883 \
  -mqtt-user plankroad \
  -mqtt-pass mqtt2026 \
  -mqtt-topic plankroad/data/

# 生成7天历史数据
./simulator -backfill 168 -interval 10s
```

### 岩性配置

| 岩性 | 孔隙率 | 断裂韧性 | 冻融系数 | 适用场景 |
|------|--------|----------|----------|----------|
| **limestone** (石灰岩) | 3% | 1.5 MPa√m | 1.2x | 明月峡、褒斜道、阴平道 |
| **sandstone** (砂岩) | 15% | 1.2 MPa√m | 1.8x | 金牛道、荔枝道 |
| **granite** (花岗岩) | 1% | 2.0 MPa√m | 0.7x | 石门栈道 |
| **gneiss** (片麻岩) | 2% | 1.8 MPa√m | 0.9x | 子午道、傥骆道 |
| **shale** (页岩) | 10% | 0.9 MPa√m | 2.0x | 米仓道 |
| **quartzite** (石英岩) | 0.5% | 2.5 MPa√m | 0.5x | 最耐风化 |

### 气候配置

| 气候 | 年均温 | 湿度 | 年冻融天数 | 适用场景 |
|------|--------|------|------------|----------|
| **temperate** (温带) | 15°C | 70% | 30天 | 大多数遗址 |
| **subtropical** (亚热带) | 20°C | 80% | 10天 | 低海拔南部遗址 |
| **alpine** (高山) | 5°C | 60% | 120天 | 高海拔遗址 (傥骆道、阴平道) |
| **arid** (干旱) | 12°C | 40% | 40天 | 干燥少雨区域 |

### 命令行参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `-api` | string | http://localhost:8080/api/sensor/batch | 后端API地址 |
| `-mqtt` | string | tcp://localhost:1883 | MQTT Broker地址 |
| `-mqtt-user` | string | - | MQTT用户名 |
| `-mqtt-pass` | string | - | MQTT密码 |
| `-mqtt-topic` | string | plankroad/data/ | MQTT主题前缀 |
| `-mqtt-enable` | bool | false | 启用MQTT上报 |
| `-interval` | duration | 1h | 上报间隔 |
| `-beams` | int | 5 | 每遗址梁数 |
| `-backfill` | int | 0 | 历史数据回填小时数 |
| `-rock-type` | string | - | 岩性覆盖 (6种可选) |
| `-climate` | string | - | 气候覆盖 (4种可选) |
| `-anomaly` | float | 0.05 | 异常注入概率 |
| `-sites` | string | 1,2,3,4,5,6,7,8,9,10 | 启用的遗址ID |

### 环境变量

| 变量 | 说明 |
|------|------|
| `BACKEND_URL` | 后端URL |
| `MQTT_BROKER` | MQTT Broker地址 |
| `MQTT_USERNAME` | MQTT用户名 |
| `MQTT_PASSWORD` | MQTT密码 |
| `MQTT_DATA_TOPIC` | MQTT数据主题 |
| `SIM_INTERVAL_SEC` | 上报间隔(秒) |
| `SIM_SITES` | 遗址ID列表 |
| `SIM_ROCK_TYPE` | 岩性覆盖 |
| `SIM_CLIMATE` | 气候覆盖 |
| `SIM_ANOMALY_CHANCE` | 异常注入概率 |

## API 接口

### 遗址管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/sites` | 获取所有遗址列表 |
| GET | `/api/sites/:id` | 获取单个遗址详情 |

### 传感器数据
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/sensor/batch` | 批量上报传感器数据 |
| POST | `/api/sensor/single` | 单条上报传感器数据 |
| GET | `/api/sites/:id/readings` | 获取遗址最近读数 |

### 结构仿真
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/sites/:id/simulate` | 运行单遗址结构仿真 |
| POST | `/api/simulate/all` | 运行所有遗址仿真 |
| GET | `/api/sites/:id/simulation` | 获取最新仿真结果 |

### 风化评估
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/sites/:id/weathering` | 运行单遗址风化评估 |
| POST | `/api/weathering/all` | 运行所有遗址评估 |
| GET | `/api/sites/:id/weathering` | 获取最新评估结果 |

### 告警管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/alarms` | 获取告警列表 |
| POST | `/api/alarms/:id/ack` | 确认告警 |
| POST | `/api/alarms/:id/resolve` | 解决告警 |

### 监控
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/dashboard` | 仪表板汇总数据 |
| GET | `/api/sites/:id/summary` | 遗址每日汇总 |
| GET | `/metrics` | Prometheus指标 |
| GET | `/debug/pprof/` | pprof性能分析 |

## 数据库策略

### 数据保留策略

| 数据类型 | 保留时长 | 说明 |
|----------|----------|------|
| 原始传感器读数 | **3个月** | 每小时10遗址×5梁=50条/小时 |
| 小时级汇总 | **1年** | 连续聚合，用于趋势分析 |
| 天级汇总 | **5年** | 用于长期历史分析 |
| 周级汇总 | **永久** | 用于年度趋势 |
| 月级汇总 | **永久** | 用于年代际分析 |

### 压缩策略

- **传感器数据**: 7天后自动压缩 (segmentby: site_id, beam_id)
- **告警数据**: 30天后自动压缩 (segmentby: site_id, alarm_type)
- 压缩比通常可达 5-10x，节省大量存储空间

### 连续聚合视图

```sql
sensor_hourly_summary   -- 每15分钟刷新
sensor_daily_summary    -- 每小时刷新
sensor_weekly_summary   -- 每天刷新
sensor_monthly_summary  -- 每周刷新
```

## 监控指标

### Prometheus 指标

```
plankroad_http_requests_total          # HTTP请求总数
plankroad_http_request_duration_seconds # HTTP请求耗时
plankroad_http_active_connections      # 活跃连接数
plankroad_sensor_readings_total         # 传感器读数总数
plankroad_fem_simulations_total         # 有限元仿真次数
plankroad_weathering_evaluations_total  # 风化评估次数
plankroad_alarms_generated_total        # 生成告警数
plankroad_alarms_published_total        # 推送告警数
plankroad_mqtt_messages_sent_total      # MQTT消息数
plankroad_bus_events_published_total    # 总线发布事件数
plankroad_bus_events_handled_total      # 总线处理事件数
```

### PProf 性能分析

```bash
# 查看CPU使用
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 查看内存使用
go tool pprof http://localhost:6060/debug/pprof/heap

# 查看goroutine
go tool pprof http://localhost:6060/debug/pprof/goroutine

# 火焰图
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/profile?seconds=60
```

## 配置调优

### 有限元配置

```dotenv
FEM_ELEMENT_SIZE=0.1         # 单元大小(m)，越小精度越高但越慢
FEM_MAX_ITERATIONS=1000      # 最大迭代次数
FEM_TOLERANCE=1e-6           # 收敛容差
FEM_CONCURRENCY=3            # 并发仿真数
FEM_YIELD_STRENGTH_WOOD=40   # 木材屈服强度(MPa)
FEM_YIELD_STRENGTH_ROCK=60   # 岩体屈服强度(MPa)
```

### 告警阈值

```dotenv
ALARM_STRAIN_THRESHOLD=1500   # 应变告警阈值(με)
ALARM_STRAIN_CRITICAL=2500    # 应变危急阈值
ALARM_CRACK_THRESHOLD=10      # 裂隙告警阈值(mm)
ALARM_CRACK_CRITICAL=30       # 裂隙危急阈值
```

### Gzip压缩

```dotenv
GZIP_ENABLED=true
GZIP_LEVEL=5                 # 1-9, 5是速度/压缩比平衡
GZIP_MIN_SIZE=1024           # 小于1KB不压缩
```

## 常见问题

### Q: 数据库连接失败?
A: 确保TimescaleDB已启动并初始化完成，检查.env中的数据库配置。

### Q: MQTT消息无法发布?
A: 检查mosquitto配置，确认用户名密码正确，1883端口未被占用。

### Q: 有限元仿真很慢?
A: 调大`FEM_ELEMENT_SIZE`或减小`FEM_MAX_ITERATIONS`，或增加`FEM_CONCURRENCY`。

### Q: 如何重置数据库?
A: `docker-compose down -v && docker-compose up -d timescale` (⚠️ 所有数据将丢失)

### Q: 如何查看服务日志?
A: 
```bash
docker-compose logs -f backend
docker-compose logs -f simulator
docker-compose logs timescale
```

## 生产部署建议

1. **使用独立存储卷**: 数据库数据不要放在容器层
2. **启用HTTPS**: 使用nginx或traefik做反向代理
3. **配置备份**: TimescaleDB定期备份到外部存储
4. **资源限制**: 在docker-compose中配置resources限制
5. **告警升级**: 配置Prometheus Alertmanager实现邮件/短信告警
6. **日志收集**: 使用ELK或Loki收集日志

## 许可证

本项目用于秦巴山区古代栈道遗址保护研究，非商业用途。

---

**古栈道保护，科技赋能。** 🏔️
