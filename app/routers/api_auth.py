import os
from fastapi import APIRouter

router = APIRouter(prefix="/api/auth", tags=["auth"])
PASSWORD = os.getenv("ADMIN_PASSWORD", "admin")


@router.post("")
async def verify(body: dict):
    pwd = body.get("password", "")
    if pwd == PASSWORD:
        return {"ok": True}
    return {"ok": False}
