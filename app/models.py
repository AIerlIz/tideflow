# TideFlow - ORM 模型

from datetime import datetime

from sqlalchemy import Column, Integer, String, Boolean, DateTime, Text
from sqlalchemy.sql import func

from app.database import Base


class DownloadSource(Base):
    """下载源（HTTP/FTP/Magnet URL）"""
    __tablename__ = "download_sources"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(255), nullable=False)
    url = Column(Text, nullable=False)
    source_type = Column(String(20), nullable=False, default="http")  # http / ftp / magnet
    enabled = Column(Boolean, default=True)
    created_at = Column(DateTime, default=func.now())
    updated_at = Column(DateTime, default=func.now(), onupdate=func.now())


class GlobalSetting(Base):
    """全局设置（键值对）"""
    __tablename__ = "global_settings"

    id = Column(Integer, primary_key=True, autoincrement=True)
    key = Column(String(100), unique=True, nullable=False)
    value = Column(Text, default="")


class TrafficRecord(Base):
    """流量消耗记录（按周期汇总）"""
    __tablename__ = "traffic_records"

    id = Column(Integer, primary_key=True, autoincrement=True)
    period_start = Column(DateTime, nullable=False)
    period_end = Column(DateTime, nullable=False)
    period_type = Column(String(20), nullable=False)           # daily / weekly / monthly
    total_bytes = Column(Integer, default=0)                   # 周期内消耗字节数
    download_count = Column(Integer, default=0)
    is_current = Column(Boolean, default=True)
    created_at = Column(DateTime, default=func.now())
