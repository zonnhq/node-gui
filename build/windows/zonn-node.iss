#define AppName "Zonn Node"
#define AppPublisher "Zonn"
#define AppExeName "zonn-node.exe"
#ifndef AppVersion
  #define AppVersion "0.2.0-dev"
#endif
; Build with /DNvidia for the NVIDIA installer, which ships the CUDA runner
; (a folder) instead of the standard one (a single file). Both install to the
; same place under the same AppId, so one replaces the other.
#ifndef SourceDir
  #define SourceDir "..\bin"
#endif
#ifndef IconFile
  #define IconFile "..\zonn-node.ico"
#endif

[Setup]
AppId={{B72E0AA4-CE30-4F1A-A64D-D81455A59589}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
DefaultDirName={localappdata}\Programs\Zonn Node
DefaultGroupName=Zonn Node
DisableProgramGroupPage=yes
OutputDir=..\installer
#ifdef Nvidia
OutputBaseFilename=Zonn-Node-Setup-Windows-x64-NVIDIA
#else
OutputBaseFilename=Zonn-Node-Setup-Windows-x64
#endif
Compression=lzma2
SolidCompression=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
WizardStyle=modern
UninstallDisplayName=Zonn Node
SetupIconFile={#IconFile}
UninstallDisplayIcon={app}\{#AppExeName}
; Close a running Zonn Node (it stays in the tray) before replacing its files.
CloseApplications=force
RestartApplications=no
; Leftovers from the other variant, so switching between them stays clean.
[InstallDelete]
Type: files; Name: "{app}\inference-worker.exe"
Type: filesandordirs; Name: "{app}\inference-worker"
Type: files; Name: "{app}\inference_worker.py"

[Files]
Source: "{#SourceDir}\zonn-node.exe"; DestDir: "{app}"; Flags: ignoreversion
#ifdef Nvidia
Source: "{#SourceDir}\inference-worker\*"; DestDir: "{app}\inference-worker"; Flags: ignoreversion recursesubdirs createallsubdirs
#else
Source: "{#SourceDir}\inference-worker.exe"; DestDir: "{app}"; Flags: ignoreversion
#endif

[Icons]
Name: "{group}\Zonn Node"; Filename: "{app}\{#AppExeName}"
Name: "{autodesktop}\Zonn Node"; Filename: "{app}\{#AppExeName}"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional shortcuts:"

[Run]
Filename: "{app}\{#AppExeName}"; Description: "Launch Zonn Node"; Flags: nowait postinstall skipifsilent

; Zonn Node keeps running in the tray after its window closes. Starting at
; sign in is a value the app writes under the current user's Run key; the
; uninstaller removes it, and stops a running Zonn Node first.
[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: none; ValueName: "Zonn Node"; Flags: uninsdeletevalue dontcreatekey

[UninstallRun]
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM {#AppExeName}"; Flags: runhidden; RunOnceId: "StopZonnNode"
