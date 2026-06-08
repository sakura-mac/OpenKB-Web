#!/usr/bin/env bash
# OKB Web 卸载脚本（macOS / Linux 通用）
#
# 模式：
#   ./uninstall.sh           交互式：询问保留/删除数据
#   ./uninstall.sh --keep    温柔模式：保留 spaces 和 config，只删 runtime/cache
#   ./uninstall.sh --purge   彻底模式：删除全部 OKB 数据（含 spaces 和 config）
#   ./uninstall.sh --yes     非交互（配合 --keep 或 --purge 使用）
#
# 退出码：
#   0  成功
#   1  用户取消
#   2  参数错误

set -euo pipefail

# ----- 颜色 -----
if [[ -t 1 ]]; then
    C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'
    C_BLUE=$'\033[34m'; C_BOLD=$'\033[1m'; C_DIM=$'\033[2m'; C_RESET=$'\033[0m'
else
    C_RED=''; C_GREEN=''; C_YELLOW=''; C_BLUE=''; C_BOLD=''; C_DIM=''; C_RESET=''
fi

info()  { echo "${C_BLUE}ℹ${C_RESET}  $*"; }
warn()  { echo "${C_YELLOW}⚠${C_RESET}  $*"; }
ok()    { echo "${C_GREEN}✓${C_RESET}  $*"; }
err()   { echo "${C_RED}✗${C_RESET}  $*" >&2; }
title() { echo; echo "${C_BOLD}$*${C_RESET}"; }

# ----- 参数解析 -----
MODE=""        # keep | purge | (空表示交互)
ASSUME_YES=0
for arg in "$@"; do
    case "$arg" in
        --keep)  MODE="keep" ;;
        --purge) MODE="purge" ;;
        --yes|-y) ASSUME_YES=1 ;;
        --help|-h)
            sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *)
            err "未知参数：$arg"
            echo "用法：$0 [--keep|--purge] [--yes]"
            exit 2
            ;;
    esac
done

# ----- 确定 OKB 数据目录 -----
# 与 internal/config/config.go 的 os.UserConfigDir() 行为对齐
case "$(uname -s)" in
    Darwin)
        OKB_HOME="${HOME}/Library/Application Support/OKB"
        ;;
    Linux)
        OKB_HOME="${XDG_CONFIG_HOME:-$HOME/.config}/OKB"
        ;;
    *)
        err "不支持的操作系统：$(uname -s)"
        exit 2
        ;;
esac

# 用户也可能用过 OKB_HOME 环境变量自定义路径
if [[ -n "${OKB_HOME_ENV:-}" ]]; then
    OKB_HOME="$OKB_HOME_ENV"
fi

title "🗑️  OKB Web 卸载向导"
echo "${C_DIM}OKB 数据目录: ${OKB_HOME}${C_RESET}"
echo

# ----- 1. 停掉运行中的进程 -----
title "[1/3] 停止运行中的 okb-web 进程"
if pgrep -f 'okb-web' >/dev/null 2>&1; then
    warn "检测到正在运行的 okb-web，正在停止..."
    pkill -f 'okb-web' || true
    sleep 1
    if pgrep -f 'okb-web' >/dev/null 2>&1; then
        warn "首次未停止，强制结束 (kill -9)..."
        pkill -9 -f 'okb-web' || true
        sleep 1
    fi
    if pgrep -f 'okb-web' >/dev/null 2>&1; then
        err "无法停止 okb-web 进程，请手动停止后重试"
        exit 1
    fi
    ok "已停止"
else
    ok "未运行（跳过）"
fi

# ----- 2. 检查数据目录 -----
title "[2/3] 检查 OKB 数据"
if [[ ! -d "$OKB_HOME" ]]; then
    info "OKB 数据目录不存在，无需清理：$OKB_HOME"
    SPACES_INFO="—"
    RUNTIME_INFO="—"
    CONFIG_INFO="—"
else
    # 统计各部分大小（du 在不同平台行为略有差异，统一 -sh）
    safe_size() {
        if [[ -d "$1" ]]; then
            du -sh "$1" 2>/dev/null | awk '{print $1}'
        else
            echo "—"
        fi
    }
    SPACES_INFO=$(safe_size "$OKB_HOME/spaces")
    RUNTIME_INFO=$(safe_size "$OKB_HOME/runtime")
    CACHE_INFO=$(safe_size "$OKB_HOME/cache")
    CONFIG_INFO="—"
    [[ -f "$OKB_HOME/config.json" ]] && CONFIG_INFO="存在"

    # 列出 spaces 数量
    SPACE_COUNT=0
    if [[ -d "$OKB_HOME/spaces" ]]; then
        SPACE_COUNT=$(find "$OKB_HOME/spaces" -maxdepth 1 -mindepth 1 -type d 2>/dev/null | wc -l | tr -d ' ')
    fi

    echo
    echo "  ${C_BOLD}你的笔记数据${C_RESET}"
    echo "    spaces  : ${SPACES_INFO}  (${SPACE_COUNT} 个空间)"
    echo "    config  : ${CONFIG_INFO}"
    echo
    echo "  ${C_BOLD}可重新下载的运行时${C_RESET}"
    echo "    runtime : ${RUNTIME_INFO}  (uv + OpenKB Python 环境)"
    echo "    cache   : ${CACHE_INFO}"
fi

# ----- 3. 决定卸载模式 -----
title "[3/3] 选择卸载方式"
if [[ -z "$MODE" ]]; then
    if [[ $ASSUME_YES -eq 1 ]]; then
        err "--yes 必须配合 --keep 或 --purge 使用"
        exit 2
    fi
    cat <<EOF

  ${C_GREEN}1)${C_RESET} 温柔卸载（推荐）
     删除：runtime + cache（约 ${RUNTIME_INFO}）
     保留：spaces + config（你的笔记和 LLM 配置）
     ${C_DIM}下次重装直接用，笔记还在${C_RESET}

  ${C_RED}2)${C_RESET} 彻底卸载
     删除：${C_BOLD}全部${C_RESET}（${OKB_HOME}）
     ${C_DIM}所有笔记和配置都会被删${C_RESET}

  ${C_DIM}3) 取消${C_RESET}

EOF
    read -p "请选择 [1/2/3]: " CHOICE
    case "$CHOICE" in
        1) MODE="keep"  ;;
        2) MODE="purge" ;;
        3|"") info "已取消"; exit 1 ;;
        *) err "无效选择"; exit 2 ;;
    esac
fi

# ----- 二次确认（彻底模式） -----
if [[ "$MODE" == "purge" && $ASSUME_YES -ne 1 && $SPACE_COUNT -gt 0 ]]; then
    echo
    warn "即将删除 ${SPACE_COUNT} 个空间的所有笔记，${C_BOLD}此操作不可恢复${C_RESET}！"
    read -p "确认删除？输入 'YES' 继续: " CONFIRM
    if [[ "$CONFIRM" != "YES" ]]; then
        info "已取消"
        exit 1
    fi
fi

# ----- 执行删除 -----
echo
case "$MODE" in
    keep)
        info "温柔卸载：删除 runtime + cache，保留 spaces + config"
        [[ -d "$OKB_HOME/runtime" ]] && rm -rf "$OKB_HOME/runtime" && ok "已删除 runtime/"
        [[ -d "$OKB_HOME/cache"   ]] && rm -rf "$OKB_HOME/cache"   && ok "已删除 cache/"
        echo
        ok "${C_GREEN}卸载完成${C_RESET}"
        echo "  保留的数据: $OKB_HOME"
        echo "  ${C_DIM}下次重新下载二进制即可继续使用${C_RESET}"
        ;;
    purge)
        info "彻底卸载：删除整个 OKB 目录"
        if [[ -d "$OKB_HOME" ]]; then
            rm -rf "$OKB_HOME"
            ok "已删除 $OKB_HOME"
        fi
        echo
        ok "${C_GREEN}卸载完成${C_RESET}"
        echo "  ${C_DIM}所有 OKB 数据已清除，无任何残留${C_RESET}"
        ;;
esac

# ----- 提示删除主程序 -----
echo
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
echo "${C_DIM}主程序解压目录可手动删除：${C_RESET}"
echo "  rm -rf '${SCRIPT_DIR}'"
