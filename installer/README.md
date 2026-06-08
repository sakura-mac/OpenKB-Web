# Windows Installer

`windows.iss` 是 Inno Setup 6 的脚本，由 `.github/workflows/release.yml` 在 `windows-latest` runner 上调用编译，产出 `OpenKB-Web-Setup-vX.Y.Z.exe`。

## 本地编译（可选，用于调试 .iss 改动）

需要 Windows + 已装 Inno Setup 6（https://jrsoftware.org/isdl.php）。

```cmd
:: 在仓库根目录
cd installer
iscc /DAppVersion=v0.1.2 /DSourceDir=..\dist\OpenKB-Web-v0.1.2-windows-amd64 windows.iss
```

输出 `installer\OpenKB-Web-Setup-v0.1.2.exe`。

## 图标

`icon.ico` 不在 git 里，由 CI 在编译时从 `web/public/favicon.svg` 用 ImageMagick 转出。本地编译前需手动转一次：

```cmd
magick convert -background none -density 384 ..\web\public\favicon.svg ^
        -define icon:auto-resize=256,128,64,48,32,16 icon.ico
```

## 安装包行为

- 默认装到 `%LocalAppData%\Programs\OpenKB-Web`（用户级，无 UAC 弹窗）
- 开始菜单：「OpenKB Web」「OKB Web Documentation」「Clear OKB Data」「Uninstall OpenKB Web」
- 桌面快捷方式：可选（默认不勾）
- 注册到控制面板「程序和功能」
- **卸载时只删程序文件**——`%AppData%\OKB\` 下的笔记/配置/运行时一概不动
- 想清数据：开始菜单→「Clear OKB Data」（跑 `uninstall.bat`，交互式选温柔/彻底）

## 不签名说明

未购买 Authenticode 证书。用户首次运行可能遇到 SmartScreen 警告：

> Windows 已保护你的电脑

点「**更多信息**」→「**仍要运行**」即可。这个警告会随该 setup.exe 累计下载次数（SmartScreen 信誉）逐渐变弱。
