#!/usr/bin/env python3
"""
DropLAN 文件互传服务
局域网内手机/PC 浏览器访问即可上传文件到电脑，或浏览/下载已上传文件。
支持 PC + 手机响应式布局。

启动: python3 server.py
访问: 浏览器打开 http://<本机IP>:8000

目录结构:
  server.py     路由 + API + 启动
  config.py     配置（保存目录 / 端口）
  static/       CSS + JS
  templates/    HTML 模板
"""

from __future__ import annotations

import mimetypes
import socket
from datetime import datetime
from pathlib import Path
from typing import List, Optional

import uvicorn
from fastapi import FastAPI, UploadFile, File, Form, Query, Request
from fastapi.responses import JSONResponse, FileResponse
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates

from config import SAVE_DIR, HOST, PORT

BASE_DIR = Path(__file__).parent

app = FastAPI()
app.mount("/static", StaticFiles(directory=BASE_DIR / "static"), name="static")
templates = Jinja2Templates(directory=BASE_DIR / "templates")


# ----------------------------------------------------------------------
# 路由：页面
# ----------------------------------------------------------------------
@app.get("/")
async def index(request: Request):
    return templates.TemplateResponse("home.html", {"request": request})


@app.get("/upload")
async def upload_page(request: Request):
    return templates.TemplateResponse("upload.html", {"request": request})


@app.get("/browse")
async def browse_page(request: Request):
    return templates.TemplateResponse("browse.html", {"request": request})


@app.get("/favicon.ico")
async def favicon():
    return FileResponse(BASE_DIR / "static" / "favicon.svg", media_type="image/svg+xml")


# ----------------------------------------------------------------------
# 路由：上传 API
# ----------------------------------------------------------------------
@app.post("/upload")
async def upload(
    files: List[UploadFile] = File(...),
    device: str = Form("default"),
):
    # 安全的设备名：只保留字母数字中文-_，防止路径穿越
    safe_device = "".join(c for c in device if c.isalnum() or c in "-_") or "default"
    device_dir = SAVE_DIR / safe_device
    device_dir.mkdir(parents=True, exist_ok=True)

    results = []
    for f in files:
        now = datetime.now()
        ts = now.strftime("%Y%m%d_%H%M%S") + f"{now.microsecond // 1000:03d}"
        # 安全的原始文件名
        raw = Path(f.filename).name if f.filename else "file"
        filename = f"{ts}_{raw}"
        path = device_dir / filename

        size = 0
        with open(path, "wb") as out:
            while True:
                chunk = await f.read(1024 * 1024)
                if not chunk:
                    break
                out.write(chunk)
                size += len(chunk)

        results.append({"name": filename, "size": size})
        print(
            f"[{datetime.now().strftime('%H:%M:%S')}] "
            f"✅ {safe_device} | {filename} | {size / 1048576:.2f}MB"
        )

    return JSONResponse({"ok": True, "files": results, "dir": str(device_dir)})


# ----------------------------------------------------------------------
# 路由：读取 API
# ----------------------------------------------------------------------
def _safe_subpath(base: Path, name: str) -> Optional[Path]:
    """校验 name 合法并返回绝对路径，含路径穿越或隐藏项则返回 None。"""
    if not name or name.startswith('.') or "/" in name or "\\" in name:
        return None
    target = (base / name).resolve()
    try:
        target.relative_to(SAVE_DIR.resolve())
    except ValueError:
        return None
    return target


@app.get("/api/dirs")
async def list_dirs():
    SAVE_DIR.mkdir(parents=True, exist_ok=True)
    dirs = []
    for d in sorted(SAVE_DIR.iterdir(), key=lambda x: x.name):
        if d.is_dir() and not d.name.startswith('.'):
            count = sum(1 for p in d.iterdir() if p.is_file() and not p.name.startswith('.'))
            dirs.append({"name": d.name, "count": count})
    return {"dirs": dirs}


@app.get("/api/files")
async def list_files(dir: str = Query(...)):
    d = _safe_subpath(SAVE_DIR, dir)
    if d is None or not d.exists() or not d.is_dir():
        return JSONResponse({"error": "目录不存在或非法"}, status_code=400)
    files = []
    for f in d.iterdir():
        if f.is_file() and not f.name.startswith('.'):
            st = f.stat()
            files.append({"name": f.name, "size": st.st_size, "mtime": int(st.st_mtime)})
    files.sort(key=lambda x: x["mtime"], reverse=True)
    return {"dir": dir, "files": files}


@app.get("/api/download")
async def download(dir: str = Query(...), file: str = Query(...)):
    d = _safe_subpath(SAVE_DIR, dir)
    if d is None or not d.exists() or not d.is_dir():
        return JSONResponse({"error": "目录不存在或非法"}, status_code=400)
    f = _safe_subpath(d, file)
    if f is None or not f.exists() or not f.is_file():
        return JSONResponse({"error": "文件不存在或非法"}, status_code=400)
    media_type = mimetypes.guess_type(f.name)[0] or "application/octet-stream"
    return FileResponse(f, filename=f.name, media_type=media_type)


# ----------------------------------------------------------------------
# 启动
# ----------------------------------------------------------------------
def get_lan_ip() -> str:
    """获取本机局域网 IP（跨平台，socket 连接法，不依赖系统命令）。"""
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        # 不真正发包，仅让系统按路由表选定出口网卡
        s.connect(("8.8.8.8", 80))
        return s.getsockname()[0]
    except Exception:
        return "<本机IP>"
    finally:
        s.close()


if __name__ == "__main__":
    SAVE_DIR.mkdir(parents=True, exist_ok=True)
    ip = get_lan_ip()
    print()
    print(f"  📁 保存目录: {SAVE_DIR}")
    print(f"  🌐 访问地址: http://{ip}:{PORT}")
    print(f"     (同局域网内，浏览器打开上面地址)")
    print(f"  🚀 启动中... 按 Ctrl+C 停止")
    print()
    uvicorn.run(app, host=HOST, port=PORT, log_level="warning")
