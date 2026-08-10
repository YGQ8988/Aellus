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

import logging
import mimetypes
import os
import socket
import tempfile
import zipfile
from datetime import datetime
from pathlib import Path
from typing import List, Optional

import uvicorn
from fastapi import FastAPI, UploadFile, File, Form, Query, Request
from fastapi.responses import JSONResponse, FileResponse
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates
from starlette.background import BackgroundTask

from config import (
    BASE_DIR,
    SAVE_DIR,
    HOST,
    PORT,
    ACCESS_LOG,
    ACCESS_LOG_DATEFMT,
    ACCESS_LOG_TEMPLATE,
    OPERATION_LOG,
    OPERATION_LOG_DATEFMT,
)

app = FastAPI()
app.mount("/static", StaticFiles(directory=BASE_DIR / "static"), name="static")
templates = Jinja2Templates(directory=BASE_DIR / "templates")

# ----------------------------------------------------------------------
# 访问日志：记录 来源IP / 请求方式 / 路径 / 响应状态 / 浏览器UA，写入 access.log
# 配置见 config.py
# ----------------------------------------------------------------------
_access_logger = logging.getLogger("droplan.access")
_access_logger.setLevel(logging.INFO)
_h = logging.FileHandler(ACCESS_LOG, encoding="utf-8")
_h.setFormatter(logging.Formatter("%(asctime)s  %(message)s", datefmt=ACCESS_LOG_DATEFMT))
_access_logger.addHandler(_h)

# ----------------------------------------------------------------------
# 操作日志：记录 上传成功 / 批量打包下载，写入 operation.log
# 配置见 config.py
# ----------------------------------------------------------------------
_op_logger = logging.getLogger("droplan.operation")
_op_logger.setLevel(logging.INFO)
_op_h = logging.FileHandler(OPERATION_LOG, encoding="utf-8")
_op_h.setFormatter(logging.Formatter("%(asctime)s  %(message)s", datefmt=OPERATION_LOG_DATEFMT))
_op_logger.addHandler(_op_h)


def _client_ip(request: Request) -> str:
    """获取真实来源 IP：优先 X-Forwarded-For 首段（反代场景），否则直连 IP。"""
    xff = request.headers.get("x-forwarded-for", "")
    if xff:
        return xff.split(",")[0].strip()
    return request.client.host if request.client else "-"


@app.middleware("http")
async def access_log_middleware(request: Request, call_next):
    response = await call_next(request)
    path = request.url.path
    # 静态资源（CSS/JS/图标）不记录，避免每次页面加载刷屏
    if not path.startswith("/static/"):
        ip = _client_ip(request)
        ua = request.headers.get("user-agent", "-")
        _access_logger.info(ACCESS_LOG_TEMPLATE.format(
            ip=ip, method=request.method, path=path,
            status=response.status_code, ua=ua,
        ))
    return response


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
        _op_logger.info(f"✅ {safe_device} | {filename} | {size / 1048576:.2f}MB")

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
async def download(dir: str = Query(...), file: str = Query(...), inline: bool = False):
    d = _safe_subpath(SAVE_DIR, dir)
    if d is None or not d.exists() or not d.is_dir():
        return JSONResponse({"error": "目录不存在或非法"}, status_code=400)
    f = _safe_subpath(d, file)
    if f is None or not f.exists() or not f.is_file():
        return JSONResponse({"error": "文件不存在或非法"}, status_code=400)
    media_type = mimetypes.guess_type(f.name)[0] or "application/octet-stream"
    # inline=1 预览模式：不传 filename，避免 Content-Disposition: attachment，浏览器内联渲染
    if inline:
        return FileResponse(f, media_type=media_type)
    return FileResponse(f, filename=f.name, media_type=media_type)


@app.post("/api/download-batch")
async def download_batch(request: Request):
    """批量打包下载：files 为空则打包目录下全部非隐藏文件。返回 zip。"""
    try:
        data = await request.json()
    except Exception:
        return JSONResponse({"error": "请求格式错误"}, status_code=400)
    dir_name = data.get("dir", "") or ""
    selected = data.get("files") or []

    d = _safe_subpath(SAVE_DIR, dir_name)
    if d is None or not d.exists() or not d.is_dir():
        return JSONResponse({"error": "目录不存在或非法"}, status_code=400)

    # 确定要打包的文件：selected 为空取全部非隐藏文件；否则逐个校验
    if not selected:
        files_to_zip = [f for f in d.iterdir() if f.is_file() and not f.name.startswith('.')]
    else:
        files_to_zip = []
        for name in selected:
            f = _safe_subpath(d, name)
            if f and f.exists() and f.is_file():
                files_to_zip.append(f)

    if not files_to_zip:
        return JSONResponse({"error": "没有可下载的文件"}, status_code=400)

    # 写入临时 zip 文件，响应结束后由 BackgroundTask 自动删除
    tmp = tempfile.NamedTemporaryFile(delete=False, suffix=".zip")
    tmp.close()
    try:
        with zipfile.ZipFile(tmp.name, 'w', zipfile.ZIP_DEFLATED) as zf:
            for f in files_to_zip:
                zf.write(f, f.name)
    except Exception:
        os.unlink(tmp.name)
        return JSONResponse({"error": "打包失败"}, status_code=500)

    _op_logger.info(f"📦 打包下载 | {dir_name} | {len(files_to_zip)} 个文件")
    return FileResponse(
        tmp.name,
        media_type="application/zip",
        filename=f"{dir_name}.zip",
        background=BackgroundTask(os.remove, tmp.name),
    )


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
