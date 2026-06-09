# TideFlow - Pydantic 数据校验模型

from datetime import datetime
from typing import Optional

from pydantic import BaseModel, Field


class SourceCreate(BaseModel):
    name: str = Field(..., min_length=1, max_length=255)
    url: str = Field(..., min_length=1)
    source_type: str = Field(default="http", pattern="^(http|ftp)$")
    enabled: bool = True


class SourceUpdate(BaseModel):
    name: Optional[str] = Field(None, min_length=1, max_length=255)
    url: Optional[str] = Field(None, min_length=1)
    source_type: Optional[str] = Field(None, pattern="^(http|ftp)$")
    enabled: Optional[bool] = None


class SourceResponse(BaseModel):
    id: int
    name: str
    url: str
    source_type: str
    enabled: bool
    created_at: datetime
    updated_at: datetime
    model_config = {"from_attributes": True}
