# TideFlow - 应用配置

import os

# 数据库路径
DATA_DIR = os.path.join(os.path.dirname(os.path.dirname(__file__)), "data")
DATABASE_URL = f"sqlite+aiosqlite:///{os.path.join(DATA_DIR, 'tideflow.db')}"

# 默认设置
DEFAULT_SETTINGS = {
    "traffic_cap_enabled": "false",
    "traffic_cap_bytes": "107374182400",   # 100GB
    "traffic_cap_period": "daily",
    "traffic_cap_reset_day": "1",
    "traffic_cap_reset_hour": "0",
    "time_window_enabled": "false",
    "time_window_start": "00:00",
    "time_window_end": "23:59",
    "default_max_speed": "0",
    "max_concurrent": "3",
    "poll_interval": "2",
}
