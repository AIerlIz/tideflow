# 🌊 TideFlow（潮汐流量）

定时消耗服务器带宽，像潮汐一样让流量定时涨落。

## 功能

- **自动下载** — 从 HTTP/HTTPS 源流式下载，数据直接丢弃
- **流量上限** — 按日/周/月设置流量上限，到达后自动暂停
- **下载时段** — 指定允许下载的时间窗口（支持跨天）
- **并发控制** — 可调并发数，实时生效
- **限速** — 单任务速度限制
- **失败冷却** — 源连续失败后自动冷却，避免无效重试
- **Web 面板** — Vue.js SPA 管理界面，密码保护

## 快速开始

```bash
# 构建
go build -o tideflow .

# 运行
./tideflow
```

打开 http://localhost:8000 ，默认密码 `admin`。

首次启动会自动添加 3 个测速下载源。

## Docker

```bash
# 构建运行
docker-compose up -d

# 或直接用 Docker
docker run -d \
  -p 8000:8000 \
  -v ./data:/app/data \
  -e ADMIN_PASSWORD=admin \
  aierliz/tideflow:latest
```

镜像支持 `amd64` / `arm64` / `arm/v7`。

## API

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/health` | 健康检查 |
| GET | `/api/version` | 版本号 |
| POST | `/api/auth` | 密码验证 |
| GET/PUT | `/api/settings` | 全局设置 |
| GET | `/api/stats` | 实时状态 |
| GET | `/api/stats/traffic` | 流量历史 |
| GET/POST/PUT/DELETE | `/api/sources` | 下载源 CRUD |
| POST | `/api/sources/test` | 测试 URL 可达性 |
| POST | `/api/sources/clear-cooldowns` | 清除冷却 |
| POST | `/api/downloads/pause` | 暂停下载 |
| POST | `/api/downloads/resume` | 恢复下载 |

## 设置项

| 设置 | 默认值 | 说明 |
|---|---|---|
| `traffic_cap_enabled` | `false` | 启用流量上限 |
| `traffic_cap_bytes` | `107374182400` | 上限字节数（100GB） |
| `traffic_cap_period` | `daily` | 重置周期：`daily` / `weekly` / `monthly` |
| `traffic_cap_reset_hour` | `0` | 重置时刻（0-23） |
| `traffic_cap_reset_weekday` | `1` | 每周重置星期（1=周一…7=周日） |
| `traffic_cap_reset_day` | `1` | 每月重置日期（1-28） |
| `time_window_enabled` | `false` | 启用下载时段 |
| `time_window_start` | `00:00` | 时段开始 |
| `time_window_end` | `23:59` | 时段结束 |
| `default_max_speed` | `0` | 单任务限速（0=不限，支持 10M/512K 格式） |
| `max_concurrent` | `3` | 最大并发数 |

## 开发

```bash
# 依赖
go mod tidy

# 构建
go build -o tideflow .

# 带版本信息构建
VERSION=$(cat VERSION)
go build -ldflags="-s -w \
  -X 'main.version=${VERSION}' \
  -X 'main.commitSHA=$(git rev-parse --short HEAD)' \
  -X 'main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)'" \
  -o tideflow .
```

## 架构

```
main.go                     # 入口，路由注册
internal/
├── config/config.go        # 配置常量
├── database/database.go    # SQLite 初始化
├── models/models.go        # 数据模型
├── enforcer/enforcer.go    # 带宽消耗引擎（goroutine）
└── handlers/               # HTTP 处理器
    ├── auth.go
    ├── downloads.go
    ├── sources.go
    ├── stats.go
    └── settings.go
app/
├── static/                 # 前端静态资源
└── templates/index.html    # Vue.js SPA
```

## 许可证

MIT
