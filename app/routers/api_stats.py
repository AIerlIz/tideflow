import app.enforcer as ef
from app.enforcer import state, get_setting
from fastapi import APIRouter, Query
from sqlalchemy import select
from app.database import async_session
from app.models import TrafficRecord

def _aggregate_bytes():
    """按 source_id 聚合所有 task 的字节数"""
    result = {}
    for key, total in ef._stream_bytes.items():
        sid = ef._task_source.get(key)
        if sid is not None:
            result[sid] = result.get(sid, 0) + total
    return result


router = APIRouter(prefix="/api/stats", tags=["stats"])


@router.get("")
async def stats():
    cap_enabled = await get_setting("traffic_cap_enabled")
    cap_str = await get_setting("traffic_cap_bytes")
    try: cap = int(cap_str) if cap_enabled == "true" else 0
    except ValueError: cap = 0

    return {
        "speed": ef._download_speed,
        "traffic": ef._traffic_this_period if not (cap_enabled == "true" and cap > 0) else min(ef._traffic_this_period, cap),
        "traffic_cap": cap,
        "cap_enabled": cap_enabled == "true",
        "stream_count": len(ef._stream_tasks),
        "stream_bytes": _aggregate_bytes(),
        "tasks": [
            {"key": key, "source_id": ef._task_source.get(key), "bytes": ef._stream_bytes.get(key, 0)}
            for key in ef._stream_tasks
        ],
        "max_concurrent": await get_setting("max_concurrent"),
        "in_window": state["in_window"],
        "window_enabled": await get_setting("time_window_enabled") == "true",
        "window_start": await get_setting("time_window_start"),
        "window_end": await get_setting("time_window_end"),
        "paused_cap": state["paused_by_cap"],
        "paused_window": state["paused_by_window"],
        "cooldown_ids": state["cooldown_ids"],
        "all_failed": state["all_failed"],
    }


@router.get("/traffic")
async def history(days: int = Query(default=30, ge=1, le=365)):
    async with async_session() as s:
        r = await s.execute(
            select(TrafficRecord).order_by(TrafficRecord.period_start.desc()).limit(days)
        )
        return [{
            "start": rec.period_start.isoformat(),
            "end": rec.period_end.isoformat(),
            "type": rec.period_type,
            "bytes": rec.total_bytes,
            "count": rec.download_count,
            "current": rec.is_current,
        } for rec in r.scalars().all()]
