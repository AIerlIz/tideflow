# TideFlow（潮汐流量）— FastAPI 主入口

import asyncio
import logging
import os
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates
from starlette.requests import Request

from app.config import DATA_DIR
from app.database import init_db, async_session
from app.models import DownloadSource
from app.routers import api_downloads, api_stats, api_sources, api_settings, api_auth
from app.enforcer import run as enforcer_run
from sqlalchemy import select, func

# 日志
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(name)s] %(levelname)s: %(message)s",
)
logger = logging.getLogger("tideflow")

# 确保数据目录存在
os.makedirs(DATA_DIR, exist_ok=True)

# 后台任务引用
_enforcer_task = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期"""
    global _enforcer_task

    # 启动：初始化数据库
    logger.info("TideFlow starting up...")
    await init_db()

    # 首次启动时添加默认下载源
    async with async_session() as s:
        count = (await s.execute(select(func.count()).select_from(DownloadSource))).scalar()
        if count == 0:
            defaults = [
                DownloadSource(name="100MB 测速文件", url="http://speedtest.tele2.net/100MB.zip", source_type="http", enabled=True),
                DownloadSource(name="10MB 测速文件", url="http://speedtest.tele2.net/10MB.zip", source_type="http", enabled=True),
                DownloadSource(name="1MB 测速文件", url="http://speedtest.tele2.net/1MB.zip", source_type="http", enabled=True),
            ]
            s.add_all(defaults)
            await s.commit()
            logger.info(f"Seeded {len(defaults)} default sources")

    # 启动流量控制引擎
    _enforcer_task = asyncio.create_task(enforcer_run())

    logger.info("TideFlow is ready! (auto-download mode)")
    yield

    # 关闭：取消后台任务
    if _enforcer_task:
        _enforcer_task.cancel()
        try:
            await _enforcer_task
        except asyncio.CancelledError:
            pass
    logger.info("TideFlow shut down")


app = FastAPI(
    title="TideFlow",
    description="潮汐流量 — 定时消耗服务器带宽，像潮汐一样让流量定时涨落",
    version="1.0.0",
    lifespan=lifespan,
)

# 静态文件
static_dir = os.path.join(os.path.dirname(__file__), "static")
app.mount("/static", StaticFiles(directory=static_dir), name="static")

# 模板（使用自定义分隔符，避免与 Vue 的 {{ }} 冲突）
templates_dir = os.path.join(os.path.dirname(__file__), "templates")
templates = Jinja2Templates(
    directory=templates_dir,
    variable_start_string="{[{",
    variable_end_string="}]}",
)

# 注册 API 路由
app.include_router(api_downloads.router)
app.include_router(api_stats.router)
app.include_router(api_sources.router)
app.include_router(api_auth.router)
app.include_router(api_settings.router)


@app.get("/")
async def index(request: Request):
    """SPA 入口页面"""
    return templates.TemplateResponse("index.html", {"request": request})


@app.get("/health")
async def health():
    """健康检查"""
    return {"status": "ok", "name": "TideFlow"}
