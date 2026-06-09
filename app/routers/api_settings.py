# TideFlow - 全局设置 API

import logging
from fastapi import APIRouter
from sqlalchemy import select

from app.database import async_session
from app.models import GlobalSetting
from app.config import DEFAULT_SETTINGS

logger = logging.getLogger("tideflow.api.settings")
router = APIRouter(prefix="/api/settings", tags=["settings"])


@router.get("")
async def get_settings():
    """获取所有设置（合并默认值）"""
    settings = dict(DEFAULT_SETTINGS)

    async with async_session() as session:
        result = await session.execute(select(GlobalSetting))
        db_settings = result.scalars().all()
        for s in db_settings:
            settings[s.key] = s.value

    return {"settings": settings}


@router.put("")
async def update_settings(body: dict):
    """批量更新设置"""
    if "settings" not in body:
        return {"message": "无效的请求格式", "success": False}

    async with async_session() as session:
        for key, value in body["settings"].items():
            result = await session.execute(
                select(GlobalSetting).where(GlobalSetting.key == key)
            )
            setting = result.scalar_one_or_none()
            if setting:
                setting.value = str(value)
            else:
                setting = GlobalSetting(key=key, value=str(value))
                session.add(setting)
        await session.commit()

    return {"message": "设置已更新", "success": True}


@router.get("/defaults")
async def get_defaults():
    """获取默认设置值"""
    return {"defaults": DEFAULT_SETTINGS}
