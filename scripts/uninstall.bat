@echo off
setlocal EnableDelayedExpansion
chcp 65001 >nul 2>&1

REM OKB Web 卸载脚本 (Windows)
REM
REM 用法：
REM   uninstall.bat           交互式
REM   uninstall.bat /keep     温柔模式：保留 spaces 和 config
REM   uninstall.bat /purge    彻底模式：删除全部 OKB 数据
REM   uninstall.bat /yes      非交互（配合 /keep 或 /purge）

set "MODE="
set "ASSUME_YES=0"
:parse
if "%~1"=="" goto after_parse
if /I "%~1"=="/keep"  set "MODE=keep"  & shift & goto parse
if /I "%~1"=="/purge" set "MODE=purge" & shift & goto parse
if /I "%~1"=="/yes"   set "ASSUME_YES=1" & shift & goto parse
if /I "%~1"=="-y"     set "ASSUME_YES=1" & shift & goto parse
if /I "%~1"=="/help"  goto show_help
if /I "%~1"=="-h"     goto show_help
echo [ERROR] 未知参数: %~1
echo 用法: %~nx0 [/keep^|/purge] [/yes]
exit /b 2
:show_help
echo OKB Web 卸载脚本
echo.
echo 用法:
echo   %~nx0           交互式
echo   %~nx0 /keep     温柔模式：保留 spaces 和 config
echo   %~nx0 /purge    彻底模式：删除全部 OKB 数据
echo   %~nx0 /yes      非交互（配合 /keep 或 /purge）
exit /b 0
:after_parse

REM ----- OKB 数据目录（与 internal/config/config.go 的 os.UserConfigDir() 对齐） -----
REM Windows: %AppData%\OKB （即 %USERPROFILE%\AppData\Roaming\OKB）
set "OKB_HOME=%APPDATA%\OKB"

echo.
echo ========================================
echo   OKB Web 卸载向导
echo ========================================
echo OKB 数据目录: %OKB_HOME%
echo.

REM ----- 1. 停止 okb-web 进程 -----
echo [1/3] 停止运行中的 okb-web 进程
tasklist /FI "IMAGENAME eq okb-web.exe" 2>nul | find /I "okb-web.exe" >nul
if %ERRORLEVEL% equ 0 (
    echo   检测到正在运行的 okb-web.exe，正在停止...
    taskkill /F /IM okb-web.exe >nul 2>&1
    timeout /t 1 /nobreak >nul
    echo   [OK] 已停止
) else (
    echo   [OK] 未运行（跳过）
)
echo.

REM ----- 2. 检查数据目录 -----
echo [2/3] 检查 OKB 数据
if not exist "%OKB_HOME%" (
    echo   OKB 数据目录不存在，无需清理
    set "SPACE_COUNT=0"
    goto choose_mode
)

REM 统计 spaces 数量
set "SPACE_COUNT=0"
if exist "%OKB_HOME%\spaces" (
    for /D %%D in ("%OKB_HOME%\spaces\*") do set /a SPACE_COUNT+=1
)

set "CONFIG_INFO=不存在"
if exist "%OKB_HOME%\config.json" set "CONFIG_INFO=存在"

set "RUNTIME_INFO=不存在"
if exist "%OKB_HOME%\runtime" set "RUNTIME_INFO=存在"

set "CACHE_INFO=不存在"
if exist "%OKB_HOME%\cache" set "CACHE_INFO=存在"

echo.
echo   你的笔记数据
echo     spaces  : !SPACE_COUNT! 个空间
echo     config  : !CONFIG_INFO!
echo.
echo   可重新下载的运行时
echo     runtime : !RUNTIME_INFO!  (uv + OpenKB Python 环境)
echo     cache   : !CACHE_INFO!
echo.

:choose_mode
REM ----- 3. 选择卸载模式 -----
echo [3/3] 选择卸载方式
if not "!MODE!"=="" goto execute

echo.
echo   1) 温柔卸载（推荐）
echo      删除: runtime + cache
echo      保留: spaces + config（你的笔记和 LLM 配置）
echo.
echo   2) 彻底卸载
echo      删除: 全部（%OKB_HOME%）
echo      所有笔记和配置都会被删
echo.
echo   3) 取消
echo.
set /p CHOICE="请选择 [1/2/3]: "
if "!CHOICE!"=="1" set "MODE=keep"
if "!CHOICE!"=="2" set "MODE=purge"
if "!CHOICE!"=="3" goto cancel
if "!CHOICE!"==""  goto cancel
if "!MODE!"==""    echo [ERROR] 无效选择 & exit /b 2

REM ----- 二次确认（彻底模式） -----
if "!MODE!"=="purge" if "!ASSUME_YES!"=="0" if !SPACE_COUNT! gtr 0 (
    echo.
    echo [WARN] 即将删除 !SPACE_COUNT! 个空间的所有笔记，此操作不可恢复！
    set /p CONFIRM="确认删除？输入 YES 继续: "
    if not "!CONFIRM!"=="YES" goto cancel
)

:execute
echo.
if "!MODE!"=="keep" (
    echo [INFO] 温柔卸载：删除 runtime + cache，保留 spaces + config
    if exist "%OKB_HOME%\runtime" rmdir /S /Q "%OKB_HOME%\runtime" && echo   [OK] 已删除 runtime\
    if exist "%OKB_HOME%\cache"   rmdir /S /Q "%OKB_HOME%\cache"   && echo   [OK] 已删除 cache\
    echo.
    echo [OK] 卸载完成
    echo   保留的数据: %OKB_HOME%
    echo   下次重新下载二进制即可继续使用
)
if "!MODE!"=="purge" (
    echo [INFO] 彻底卸载：删除整个 OKB 目录
    if exist "%OKB_HOME%" rmdir /S /Q "%OKB_HOME%" && echo   [OK] 已删除 %OKB_HOME%
    echo.
    echo [OK] 卸载完成
    echo   所有 OKB 数据已清除，无任何残留
)
echo.
echo 主程序解压目录可手动删除（即本文件所在目录）：
echo   %~dp0
echo.
pause
exit /b 0

:cancel
echo.
echo [INFO] 已取消
exit /b 1
