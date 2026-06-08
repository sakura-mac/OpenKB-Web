#!/bin/bash
# okb-web 可靠启动脚本
# 用法: bash start.sh [skip-web]

set -e

DIR="$(cd "$(dirname "$0")" && pwd)"
PORT=8901
LOG="/tmp/okb-web.log"
BIN="$DIR/okb-web"
SKIP_WEB="${1:-}"

echo "=== okb-web 启动脚本 ==="

# 1. 停止占用端口的旧进程（优雅退出，避免连带杀掉父 shell/VS Code 终端）
echo "[1/4] 停止旧进程..."
OLD_PIDS=$(lsof -ti:$PORT 2>/dev/null || true)
if [ -n "$OLD_PIDS" ]; then
    # 只杀进程名为 okb-web 的进程，且排除当前 shell 自己 + 直系父进程，避免误伤 VS Code 终端
    SAFE_PIDS=""
    for p in $OLD_PIDS; do
        # 排除自身、父进程、祖父进程
        [ "$p" = "$$" ] && continue
        [ "$p" = "$PPID" ] && continue
        # 校验进程名确实是 okb-web，避免 lsof 把别的服务（同端口）也带进来
        comm=$(ps -o comm= -p "$p" 2>/dev/null | tr -d ' ')
        case "$comm" in
            okb-web|okb-web*) SAFE_PIDS="$SAFE_PIDS $p" ;;
            *) echo "  跳过 PID $p（命令=$comm，非 okb-web）" ;;
        esac
    done
    if [ -n "$SAFE_PIDS" ]; then
        echo "  优雅停止:$SAFE_PIDS"
        kill -TERM $SAFE_PIDS 2>/dev/null || true
        # 最多等 3 秒让其自己退
        for i in 1 2 3; do
            still_alive=""
            for p in $SAFE_PIDS; do
                kill -0 "$p" 2>/dev/null && still_alive="$still_alive $p"
            done
            [ -z "$still_alive" ] && break
            sleep 1
        done
        # 还活着的强制 -9
        if [ -n "$still_alive" ]; then
            echo "  强制停止:$still_alive"
            kill -9 $still_alive 2>/dev/null || true
            sleep 1
        fi
    fi
    if lsof -ti:$PORT >/dev/null 2>&1; then
        echo "❌ 端口 $PORT 仍被占用，无法启动"
        exit 1
    fi
    echo "  进程已停止"
else
    echo "  无旧进程"
fi

# 2. 编译前端（实时显示日志）
if [ "$SKIP_WEB" = "skip-web" ]; then
    echo "[2/4] 跳过前端编译"
else
    echo "[2/4] 编译前端..."
    if [ -s "$HOME/.nvm/nvm.sh" ]; then
        . "$HOME/.nvm/nvm.sh"
        nvm use 22 >/dev/null 2>&1 || true
    fi
    cd "$DIR/web"
    echo "  开始编译，实时显示日志:"
    npm run build 2>&1 | while IFS= read -r line; do
        echo "    $line"
    done
    BUILD_EXIT=${PIPESTATUS[0]}
    if [ $BUILD_EXIT -ne 0 ]; then
        echo "❌ 前端编译失败"
        exit 1
    fi
    echo "  前端编译完成"
fi

# 3. 编译后端（实时显示日志）
echo "[3/4] 编译后端..."
cd "$DIR"
echo "  开始编译，实时显示日志:"
go build -o okb-web . 2>&1 | while IFS= read -r line; do
    echo "    $line"
done
BUILD_EXIT=${PIPESTATUS[0]}
if [ $BUILD_EXIT -ne 0 ]; then
    echo "❌ 后端编译失败"
    exit 1
fi
echo "  后端编译完成"

# 4. 启动服务（实时显示日志）
echo "[4/4] 启动服务..."
# 清空旧日志
echo "" > "$LOG"
# 启动并实时显示日志
nohup "$BIN" > "$LOG" 2>&1 &
NEW_PID=$!
echo "  进程 PID: $NEW_PID"
echo "  实时日志:"

# 实时显示日志并等待端口启动
STARTED=false
for i in {1..15}; do
    # 检查进程是否存活
    if ! kill -0 $NEW_PID 2>/dev/null; then
        echo "❌ 进程已退出，查看详细日志:"
        tail -20 "$LOG"
        exit 1
    fi
    
    # 检查端口是否监听
    if lsof -ti:$PORT >/dev/null 2>&1; then
        echo "✅ 端口 $PORT 已监听"
        STARTED=true
        break
    fi
    
    # 显示最新日志
    if [ -s "$LOG" ]; then
        tail -1 "$LOG" 2>/dev/null | while IFS= read -r line; do
            echo "    $line"
        done
    fi
    
    sleep 1
    echo "  等待启动... ($i/15)"
done

if [ "$STARTED" = "false" ]; then
    echo "❌ 启动超时，查看详细日志:"
    tail -20 "$LOG"
    kill -9 $NEW_PID 2>/dev/null || true
    exit 1
fi

echo ""
echo "✅ okb-web 启动成功"
echo "   PID:  $NEW_PID"
echo "   端口: $PORT"
echo "   日志: $LOG"
echo "   地址: http://localhost:$PORT"
echo ""
echo "📋 快捷命令:"
echo "   查看实时日志: tail -f $LOG"
echo "   停止服务: kill $NEW_PID"
echo "   状态检查: curl -s http://localhost:$PORT/api/spaces"
