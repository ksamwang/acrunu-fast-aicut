#ifndef MyAppVersion
  #define MyAppVersion "0.1.0"
#endif
#ifndef SourceDir
  #error SourceDir must point to the staged Local Agent files
#endif
#ifndef OutputDir
  #define OutputDir "."
#endif

#define MyAppName "ACRUNU Fast Cut Local Agent"
#define MyAppExeName "local-agent.exe"
#define MyProtocol "acrunu-fastcut"

[Setup]
AppId={{E439989E-C89A-43C2-93AC-08D21E61A593}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher=ACRUNU
DefaultDirName={localappdata}\Programs\ACRUNU\Fast Cut Local Agent
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir={#OutputDir}
OutputBaseFilename=ACRUNU-Fast-Cut-Local-Agent-Setup-x64
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
CloseApplications=yes
CloseApplicationsFilter=local-agent.exe
RestartApplications=no
ChangesAssociations=yes
SetupLogging=yes
SetupIconFile={#SourceDir}\tray.ico
UninstallDisplayIcon={app}\tray.ico
UninstallDisplayName={#MyAppName}
VersionInfoVersion={#MyAppVersion}
VersionInfoCompany=ACRUNU
VersionInfoDescription={#MyAppName} Installer
VersionInfoProductName={#MyAppName}
VersionInfoProductVersion={#MyAppVersion}
LicenseFile={#SourceDir}\LICENSE

[Files]
Source: "{#SourceDir}\local-agent.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\tray.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\THIRD_PARTY_NOTICES.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\ffmpeg\bin\ffmpeg.exe"; DestDir: "{app}\ffmpeg\bin"; Flags: ignoreversion
Source: "{#SourceDir}\ffmpeg\bin\ffprobe.exe"; DestDir: "{app}\ffmpeg\bin"; Flags: ignoreversion
Source: "{#SourceDir}\ffmpeg\LICENSE"; DestDir: "{app}\ffmpeg"; Flags: ignoreversion
Source: "{#SourceDir}\ffmpeg\README.txt"; DestDir: "{app}\ffmpeg"; Flags: ignoreversion

[Icons]
Name: "{userstartup}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Parameters: "--autostart"; WorkingDir: "{app}"; IconFilename: "{app}\tray.ico"

[Registry]
Root: HKCU; Subkey: "Software\Classes\{#MyProtocol}"; ValueType: string; ValueName: ""; ValueData: "URL:ACRUNU Fast Cut Local Agent"; Flags: uninsdeletekey
Root: HKCU; Subkey: "Software\Classes\{#MyProtocol}"; ValueType: string; ValueName: "URL Protocol"; ValueData: ""
Root: HKCU; Subkey: "Software\Classes\{#MyProtocol}\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\tray.ico"
Root: HKCU; Subkey: "Software\Classes\{#MyProtocol}\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\{#MyAppExeName}"" --protocol ""%1"""

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "启动 {#MyAppName}"; WorkingDir: "{app}"; Flags: nowait
