# TideFlow - 下载控制 API

from fastapi import APIRouter
import app.enforcer as ef

router = APIRouter(prefix="/api/downloads", tags=["downloads"])


@router.post("/pause")
async def pause():
    await ef._stop_all()
    return {"ok": True}


@router.post("/resume")
async def resume():
    # enforcer 下个周期自动填充
    return {"ok": True}
