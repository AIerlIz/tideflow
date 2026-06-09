# TideFlow - 带宽消耗引擎
"""
纯 httpx 异步流式下载，数据直接丢弃。
规则：时段内 + 未达上限 → 随机选源，填满 slot
"""

import asyncio
import logging
import random
import time
from datetime import datetime, timedelta
from typing import Optional

import httpx
from sqlalchemy import select

from app.config import DEFAULT_SETTINGS
from app.database import async_session
from app.models import GlobalSetting, TrafficRecord, DownloadSource

logger = logging.getLogger("tideflow")

# ---- 全局状态 ----
_paused_by_cap = False
_paused_by_window = False
_task_id = 0
_stream_tasks: dict[str, asyncio.Task] = {}   # {task_key: task}
_stream_bytes: dict[str, int] = {}            # {task_key: bytes}
_task_source: dict[str, int] = {}             # {task_key: source_id}
_failure_tracker: dict[int, tuple[int, float]] = {}
_traffic_this_period: int = 0
_download_speed: int = 0
_last_bytes_total: int = 0
_last_sample_time: float = 0

state = {
    "paused_by_cap": False,
    "paused_by_window": False,
    "in_window": True,
    "cooldown_ids": [],
    "all_failed": False,
}


def cooldown(failures: int) -> int:
    if failures <= 1: return 30
    if failures == 2: return 300
    return 1800


# ---- 数据库 ----

async def get_setting(key: str) -> str:
    async with async_session() as s:
        r = await s.execute(select(GlobalSetting).where(GlobalSetting.key == key))
        v = r.scalar_one_or_none()
        return v.value if v else DEFAULT_SETTINGS.get(key, "")


async def _current_record() -> Optional[TrafficRecord]:
    async with async_session() as s:
        r = await s.execute(select(TrafficRecord).where(TrafficRecord.is_current == True))
        return r.scalar_one_or_none()


async def _sync_traffic():
    """将内存中的流量增量写入数据库"""
    global _traffic_this_period
    async with async_session() as s:
        r = await s.execute(select(TrafficRecord).where(TrafficRecord.is_current == True))
        rec = r.scalar_one_or_none()
        if rec:
            rec.total_bytes = _traffic_this_period
            rec.download_count += 1
            await s.commit()


async def _sources() -> list[DownloadSource]:
    async with async_session() as s:
        r = await s.execute(select(DownloadSource).where(DownloadSource.enabled == True))
        return list(r.scalars().all())


def _in_window(start_str: str, end_str: str) -> bool:
    now = datetime.now().time()
    try:
        sh, sm = map(int, start_str.split(":"))
        eh, em = map(int, end_str.split(":"))
    except ValueError:
        return True
    s = datetime.strptime(f"{sh:02d}:{sm:02d}", "%H:%M").time()
    e = datetime.strptime(f"{eh:02d}:{em:02d}", "%H:%M").time()
    return s <= now <= e if s <= e else (now >= s or now <= e)


async def _can_download() -> bool:
    if await get_setting("time_window_enabled") == "true":
        if not _in_window(await get_setting("time_window_start"), await get_setting("time_window_end")):
            return False
    if await get_setting("traffic_cap_enabled") == "true":
        rec = await _current_record()
        if rec:
            try: cap = int(await get_setting("traffic_cap_bytes"))
            except ValueError: cap = 0
            if cap > 0 and _traffic_this_period >= cap:
                return False
    return True


async def _init_period():
    if await _current_record():
        return
    pt = await get_setting("traffic_cap_period")
    try: rh = int(await get_setting("traffic_cap_reset_hour"))
    except ValueError: rh = 0
    try: rd = int(await get_setting("traffic_cap_reset_day"))
    except ValueError: rd = 1
    now = datetime.now()
    if pt == "daily":
        rst = now.replace(hour=rh, minute=0, second=0, microsecond=0)
        end = rst if now < rst else rst + timedelta(days=1)
    elif pt == "weekly":
        mon = (now - timedelta(days=now.weekday())).replace(hour=rh, minute=0, second=0, microsecond=0)
        end = mon if now < mon else mon + timedelta(days=7)
    elif pt == "monthly":
        try: this_m = now.replace(day=min(rd, 28), hour=rh, minute=0, second=0, microsecond=0)
        except ValueError: this_m = now.replace(day=1, hour=rh, minute=0, second=0, microsecond=0)
        end = this_m if now < this_m else (
            (now.replace(year=now.year+1, month=1) if now.month==12 else now.replace(month=now.month+1))
            .replace(day=min(rd,28), hour=rh, minute=0, second=0, microsecond=0)
        )
    else:
        end = now + timedelta(days=1)
    async with async_session() as s:
        s.add(TrafficRecord(period_start=now, period_end=end, period_type=pt, total_bytes=0, download_count=0, is_current=True))
        await s.commit()


async def _reset_period():
    global _traffic_this_period
    await _sync_traffic()
    async with async_session() as s:
        r = await s.execute(select(TrafficRecord).where(TrafficRecord.is_current == True))
        old = r.scalar_one_or_none()
        if old: old.is_current = False; old.period_end = datetime.now()
        await s.commit()
        await _init_period()
    _traffic_this_period = 0
    _failure_tracker.clear()
    state["cooldown_ids"] = []
    state["all_failed"] = False
    logger.info("Period reset")


# 用于 _stream 快速读取（避免每次 chunk 都查 DB）
_traffic_cap: int = 0

# ---- 流式下载 ----

async def _stream(task_key: str, source_id: int, url: str, name: str, limit: str = ""):
    """下载一个源，数据丢弃，实时更新 _stream_bytes"""
    global _traffic_this_period
    total = 0
    try:
        bps = 0
        if limit:
            try:
                s = limit.upper()
                if s.endswith("M"): bps = int(float(s[:-1]) * 1048576)
                elif s.endswith("K"): bps = int(float(s[:-1]) * 1024)
                else: bps = int(s)
            except ValueError: pass

        async with httpx.AsyncClient(timeout=httpx.Timeout(600, connect=30), follow_redirects=True) as cl:
            async with cl.stream("GET", url) as resp:
                resp.raise_for_status()
                t0 = time.time()
                async for chunk in resp.aiter_bytes(1048576):
                    total += len(chunk)
                    _stream_bytes[task_key] = total
                    _traffic_this_period += len(chunk)
                    # 检查上限：超出则截断，不统计超额部分
                    if _traffic_cap > 0 and _traffic_this_period > _traffic_cap:
                        excess = _traffic_this_period - _traffic_cap
                        _traffic_this_period = _traffic_cap
                        total -= excess
                        _stream_bytes[task_key] = total
                        _paused_by_cap = True
                        break
                    if bps > 0:
                        elapsed = time.time() - t0
                        expected = total / bps
                        if elapsed < expected:
                            await asyncio.sleep(expected - elapsed)

        if total > 10240:
            await _sync_traffic()
            _failure_tracker.pop(source_id, None)
            logger.info(f"✓ {name}: {total/1073741824:.1f}GB")
        else:
            raise Exception(f"too small: {total} bytes")

    except asyncio.CancelledError:
        if total > 10240: await _sync_traffic()
        raise
    except Exception as e:
        if total > 10240:
            await _sync_traffic()
            _failure_tracker.pop(source_id, None)
            logger.info(f"⚠ {name}: {total} bytes (partial)")
        else:
            fails, _ = _failure_tracker.get(source_id, (0, 0))
            fails += 1
            _failure_tracker[source_id] = (fails, time.time())
            logger.warning(f"✗ {name} ({fails}x): {e}")
    finally:
        _stream_tasks.pop(task_key, None)
        _stream_bytes.pop(task_key, None)
        _task_source.pop(task_key, None)
        # 立即补位：任务完成后马上随机选新源
        asyncio.create_task(_fill())


async def _stop_one(task_key: str):
    t = _stream_tasks.pop(task_key, None)
    _stream_bytes.pop(task_key, None)
    _task_source.pop(task_key, None)
    if t and not t.done():
        t.cancel()
        try: await t
        except asyncio.CancelledError: pass


async def _stop_all():
    for key in list(_stream_tasks):
        await _stop_one(key)


# ---- 填充槽位 ----

async def _fill():
    # 已达上限或暂停 → 不填充
    if _paused_by_cap or _paused_by_window:
        return
    if not await _can_download():
        return
    sources = await _sources()
    if not sources: return

    now = time.time()
    available, cooldown_ids = [], []
    for s in sources:
        if s.id in _failure_tracker:
            fails, ts = _failure_tracker[s.id]
            if now - ts < cooldown(fails):
                cooldown_ids.append(s.id); continue
            del _failure_tracker[s.id]
        available.append(s)  # 同源可多路并发

    state["cooldown_ids"] = cooldown_ids
    state["all_failed"] = len(sources) > 0 and len(available) == 0 and len(_stream_tasks) == 0

    if not available: return

    try: max_cc = int(await get_setting("max_concurrent"))
    except ValueError: max_cc = 3

    need = max_cc - len(_stream_tasks)
    if need <= 0: return

    pick = random.sample(available, min(need, len(available)))
    speed = await get_setting("default_max_speed")
    if speed == "0": speed = ""

    global _task_id
    for s in pick:
        _task_id += 1
        key = f"{s.id}-{_task_id}"
        logger.info(f"▶ {s.name} [{key}]")
        _stream_tasks[key] = asyncio.create_task(_stream(key, s.id, s.url, s.name, speed))
        _task_source[key] = s.id


# ---- 速度计算 ----

def _calc_speed():
    """每秒调用，计算瞬时下载速度"""
    global _download_speed, _last_bytes_total, _last_sample_time
    now = time.time()
    current = sum(_stream_bytes.values())
    if _last_sample_time > 0:
        elapsed = now - _last_sample_time
        if elapsed > 0:
            _download_speed = int((current - _last_bytes_total) / elapsed)
    _last_bytes_total = current
    _last_sample_time = now


# ---- 主循环 ----

async def run():
    global _paused_by_cap, _paused_by_window

    logger.info("TideFlow started")
    await _init_period()
    # 从 DB 恢复流量计数
    rec = await _current_record()
    if rec:
        global _traffic_this_period
        _traffic_this_period = rec.total_bytes

    while True:
        try:
            await asyncio.sleep(1)
            _calc_speed()

            # ---- 时段 ----
            tw = await get_setting("time_window_enabled")
            if tw == "true":
                in_w = _in_window(await get_setting("time_window_start"), await get_setting("time_window_end"))
                state["in_window"] = in_w
                if not in_w and not _paused_by_window:
                    await _stop_all()
                    _paused_by_window = True; state["paused_by_window"] = True
                    logger.info("Outside window → PAUSED")
                elif in_w and _paused_by_window:
                    _paused_by_window = False; state["paused_by_window"] = False
            elif _paused_by_window:
                _paused_by_window = False; state["paused_by_window"] = False

            # ---- 上限 ----
            ce = await get_setting("traffic_cap_enabled")
            if ce == "true":
                try: _traffic_cap = int(await get_setting("traffic_cap_bytes"))
                except ValueError: _traffic_cap = 0
                if _traffic_cap > 0 and _traffic_this_period >= _traffic_cap and not _paused_by_cap:
                    await _stop_all()
                    _paused_by_cap = True; state["paused_by_cap"] = True
                    logger.info(f"Cap {_traffic_this_period}/{_traffic_cap} → PAUSED")
                elif _traffic_this_period < _traffic_cap and _paused_by_cap:
                    _paused_by_cap = False; state["paused_by_cap"] = False
            else:
                _traffic_cap = 0
                if _paused_by_cap:
                    _paused_by_cap = False; state["paused_by_cap"] = False

            # ---- 填充 ----
            if await _can_download() and not _paused_by_cap and not _paused_by_window:
                await _fill()

            # ---- 周期重置 ----
            rec = await _current_record()
            if rec and datetime.now() >= rec.period_end:
                await _stop_all()
                await _reset_period()
                _paused_by_cap = False; state["paused_by_cap"] = False

        except asyncio.CancelledError:
            await _stop_all(); await _sync_traffic()
            logger.info("TideFlow stopped"); break
        except Exception as e:
            logger.error(f"Loop error: {e}", exc_info=True)
