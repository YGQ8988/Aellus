"""配置文件：保存目录、监听地址、端口。"""

from pathlib import Path

# 文件保存根目录（桌面下的 file-drops）
SAVE_DIR = Path.home() / "Desktop" / "file-drops"

# 服务监听地址（0.0.0.0 表示允许局域网访问）
HOST = "0.0.0.0"

# 服务端口
PORT = 8000
