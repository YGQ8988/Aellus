"""配置文件：保存目录、监听地址、端口、日志。"""

from pathlib import Path

# 项目根目录（config.py 所在目录）
BASE_DIR = Path(__file__).parent

# 文件保存根目录（桌面下的 file-drops）
SAVE_DIR = Path.home() / "Desktop" / "file-drops"

# 服务监听地址（0.0.0.0 表示允许局域网访问）
HOST = "0.0.0.0"

# 服务端口
PORT = 8000

# ----------------------------------------------------------------------
# 日志配置
# ----------------------------------------------------------------------

# 访问日志文件路径（记录 来源IP / 请求方式 / 路径 / 响应状态 / 浏览器UA）
ACCESS_LOG = BASE_DIR / "access.log"

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
OPERATION_LOG = BASE_DIR / "operation.log"

# 操作日志时间格式
OPERATION_LOG_DATEFMT = "%Y-%m-%d %H:%M:%S"
