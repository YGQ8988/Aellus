#!/bin/bash
# DropLAN 文件互传服务启动脚本
# 用法: ./run.sh          启动
#       ./run.sh stop     停止
#       ./run.sh status   查看状态

DIR="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$DIR/server.pid"
LOG_FILE="$DIR/server.log"

# 跨平台获取局域网 IP（macOS/Linux 通用，socket 连接法）
get_ip() {
  python3 -c "import socket; s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.connect(('8.8.8.8',80)); print(s.getsockname()[0]); s.close()" 2>/dev/null || echo '<本机IP>'
}

case "${1:-start}" in
  start)
    if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
      echo "服务已在运行 (PID: $(cat "$PID_FILE"))"
      exit 0
    fi
    nohup python3 "$DIR/server.py" > "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"
    sleep 1.5
    if kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
      echo "✅ 服务已启动 (PID: $(cat "$PID_FILE"))"
      echo "🌐 手机访问: http://$(get_ip):8000"
    else
      echo "❌ 启动失败，查看日志: $LOG_FILE"
      exit 1
    fi
    ;;
  stop)
    if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
      kill "$(cat "$PID_FILE")"
      rm -f "$PID_FILE"
      echo "✅ 服务已停止"
    else
      echo "服务未在运行"
      rm -f "$PID_FILE"
    fi
    ;;
  status)
    if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
      echo "✅ 运行中 (PID: $(cat "$PID_FILE"))"
      echo "🌐 http://$(get_ip):8000"
    else
      echo "❌ 未运行"
    fi
    ;;
  *)
    echo "用法: $0 {start|stop|status}"
    ;;
esac
