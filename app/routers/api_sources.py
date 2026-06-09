# TideFlow - 下载源管理 API

import logging

import httpx
from fastapi import APIRouter, HTTPException
from sqlalchemy import select

from app.database import async_session
from app.models import DownloadSource
from app.schemas import SourceCreate, SourceUpdate, SourceResponse
from app.enforcer import _failure_tracker, state, _stream_bytes, _task_source

logger = logging.getLogger("tideflow")
router = APIRouter(prefix="/api/sources", tags=["sources"])


@router.get("")
async def list_sources():
    async with async_session() as s:
        r = await s.execute(select(DownloadSource).order_by(DownloadSource.id))
        srcs = r.scalars().all()

    result = []
    for src in srcs:
        d = SourceResponse.model_validate(src).model_dump()
        # 聚合该源所有 task 的字节数
        total_bytes = 0
        for key, sid in _task_source.items():
            if sid == src.id:
                total_bytes += _stream_bytes.get(key, 0)
        d["downloading"] = total_bytes > 0 or any(sid == src.id for sid in _task_source.values())
        d["bytes"] = total_bytes
        d["in_cooldown"] = src.id in state.get("cooldown_ids", [])
        result.append(d)
    return result


@router.post("/test")
async def test_url(body: dict):
    """HEAD 请求验证 URL 是否可达"""
    url = body.get("url", "")
    if not url:
        raise HTTPException(400, "URL 不能为空")
    try:
        async with httpx.AsyncClient(timeout=httpx.Timeout(10, connect=5), follow_redirects=True) as cl:
            resp = await cl.head(url)
            return {"ok": True, "status": resp.status_code, "size": resp.headers.get("content-length", "?")}
    except Exception as e:
        return {"ok": False, "error": str(e)}


@router.post("", response_model=SourceResponse)
async def create_source(body: SourceCreate):
    async with async_session() as s:
        src = DownloadSource(name=body.name, url=body.url, source_type=body.source_type, enabled=body.enabled)
        s.add(src)
        await s.commit()
        await s.refresh(src)
        _failure_tracker.pop(src.id, None)
        return SourceResponse.model_validate(src)


@router.put("/{sid}", response_model=SourceResponse)
async def update_source(sid: int, body: SourceUpdate):
    async with async_session() as s:
        r = await s.execute(select(DownloadSource).where(DownloadSource.id == sid))
        src = r.scalar_one_or_none()
        if not src: raise HTTPException(404, "源不存在")
        for k, v in body.model_dump(exclude_unset=True).items():
            setattr(src, k, v)
        await s.commit()
        await s.refresh(src)
        _failure_tracker.pop(sid, None)
        return SourceResponse.model_validate(src)


@router.delete("/{sid}")
async def delete_source(sid: int):
    async with async_session() as s:
        r = await s.execute(select(DownloadSource).where(DownloadSource.id == sid))
        src = r.scalar_one_or_none()
        if not src: raise HTTPException(404, "源不存在")
        await s.delete(src); await s.commit()
    return {"ok": True}


@router.post("/clear-cooldowns")
async def clear():
    n = len(_failure_tracker)
    _failure_tracker.clear()
    state["cooldown_ids"] = []
    state["all_failed"] = False
    return {"ok": True, "cleared": n}
