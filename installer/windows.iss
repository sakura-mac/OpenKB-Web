; Inno Setup script for OpenKB Web (Windows installer)
;
; 由 .github/workflows/release.yml 调用：
;   iscc /DAppVersion=v0.1.2 /DSourceDir=dist/OpenKB-Web-vX.Y.Z-windows-amd64 \
;        installer/windows.iss
;
; 输出：dist/OpenKB-Web-Setup-v0.1.2.exe
;
; 设计要点：
; - 安装到 %LocalAppData%\Programs\OpenKB-Web（用户级，无需管理员，不弹 UAC）
; - 创建开始菜单 + 可选桌面快捷方式
; - 注册到「程序和功能」（控制面板能卸载）
; - 卸载时**只删程序文件**，不动 %AppData%\OKB（笔记数据）；
;   想清空数据让用户跑包内附带的 uninstall.bat
; - 不签名（暂未购买 Authenticode 证书）；用户首次运行 SmartScreen 会拦
;   一次"Windows 已保护你的电脑"，"更多信息→仍要运行"即可

#ifndef AppVersion
  #define AppVersion "dev"
#endif
#ifndef SourceDir
  #define SourceDir "dist\OpenKB-Web-windows-amd64"
#endif

#define AppName       "OpenKB Web"
#define AppPublisher  "sakura-mac"
#define AppURL        "https://github.com/sakura-mac/OpenKB-Web"
#define AppExeName    "okb-web.exe"

[Setup]
; AppId GUID 一旦定下不要再改——升级时靠它识别同一个产品
AppId={{C7E5D4F2-3B89-4A1F-9C74-8E6D2A1B7F50}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/issues
AppUpdatesURL={#AppURL}/releases
DefaultDirName={localappdata}\Programs\OpenKB-Web
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
DisableDirPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
OutputDir=.
OutputBaseFilename=OpenKB-Web-Setup-{#AppVersion}
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
SetupIconFile=icon.ico
UninstallDisplayIcon={app}\{#AppExeName}
UninstallDisplayName={#AppName}
; 安装结束页可勾选"立即启动"
ChangesAssociations=no

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "chinese"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
; 主程序 + 包内附带文件（uninstall.bat 用户清数据时手动跑）
Source: "{#SourceDir}\okb-web.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\README.md";   DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\LICENSE";     DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\.env.example"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\uninstall.bat"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#AppName}";          Filename: "{app}\{#AppExeName}"; WorkingDir: "{app}"
Name: "{group}\OKB Web Documentation"; Filename: "{app}\README.md"
Name: "{group}\Clear OKB Data";      Filename: "{app}\uninstall.bat"; WorkingDir: "{app}"; Comment: "清理笔记/配置/运行时数据"
Name: "{group}\{cm:UninstallProgram,{#AppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExeName}"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
; 安装后可选立即启动
Filename: "{app}\{#AppExeName}"; Description: "{cm:LaunchProgram,{#AppName}}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; 程序自身目录的临时文件等。OKB 数据在 %AppData%\OKB 不动（用户笔记）。
Type: files; Name: "{app}\*.log"
