"""配置文件：保存目录、监听地址、端口、日志。"""

import sys
from pathlib import Path

# PyInstaller 打包后资源（templates/static）在 sys._MEIPASS 临时目录；
# 开发模式下资源在脚本所在目录
if getattr(sys, 'frozen', False):
    BASE_DIR = Path(sys._MEIPASS)           # 资源目录（只读，templates/static）
    EXE_DIR = Path(sys.executable).parent   # exe 所在目录（日志写这里）
else:
    BASE_DIR = Path(__file__).parent
    EXE_DIR = BASE_DIR

# 文件保存根目录（运行时可由 server.py 覆盖，默认桌面 aellus-drops）
SAVE_DIR = Path.home() / "Desktop" / "aellus-drops"

# 服务监听地址（0.0.0.0 表示允许局域网访问）
HOST = "0.0.0.0"

# 服务端口
PORT = 8000

# ----------------------------------------------------------------------
# 日志配置（日志写在 exe 旁边或项目目录，不写进临时资源目录）
# ----------------------------------------------------------------------

# 访问日志文件路径（记录 来源IP / 请求方式 / 路径 / 响应状态 / 浏览器UA）
ACCESS_LOG = EXE_DIR / "access.log"

# 访问日志时间格式
ACCESS_LOG_DATEFMT = "%Y-%m-%d %H:%M:%S"

# 访问日志单条记录模板（各字段带中文标注，字段间以两个空格分隔）
ACCESS_LOG_TEMPLATE = (
    "访问来源IP: {ip}  "
    "请求方式: {method}  "
    "请求URL路径: {path}  "
    "响应状态: {status}  "
    '浏览器UA: "{ua}"'
)

# 操作日志文件路径（记录 上传成功 / 批量打包下载）
OPERATION_LOG = EXE_DIR / "operation.log"

# 操作日志时间格式
OPERATION_LOG_DATEFMT = "%Y-%m-%d %H:%M:%S"
