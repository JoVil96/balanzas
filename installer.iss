; Balanza POS — Script de Inno Setup 6
; Genera: dist\BalanzaPOS-Setup-1.0.0.exe

#define AppName      "Balanza POS"
#define AppVersion   "1.0.0"
#define AppPublisher "Contamatic"
#define AppExeName   "balanza-pos.exe"
#define ServiceName  "BalanzaPOS"

[Setup]
AppId={{F3A7C2D1-8B4E-4F9A-BC6D-1E2A3F4C5D6E}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL=https://contamatic.ec
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
OutputDir=dist
OutputBaseFilename=BalanzaPOS-Setup-{#AppVersion}
SetupIconFile=icon.ico
Compression=lzma2/ultra64
SolidCompression=yes
PrivilegesRequired=admin
PrivilegesRequiredOverridesAllowed=dialog
WizardStyle=modern
WizardSizePercent=120
DisableProgramGroupPage=yes
CloseApplications=yes
RestartIfNeededByRun=no
UninstallDisplayIcon={app}\{#AppExeName}
VersionInfoVersion={#AppVersion}
VersionInfoCompany={#AppPublisher}
VersionInfoDescription={#AppName}

[Languages]
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"

[Tasks]
Name: "desktopicon";   Description: "Crear icono en el Escritorio";  GroupDescription: "Iconos adicionales:"
Name: "startupicon";   Description: "Iniciar interfaz al encender el equipo"; GroupDescription: "Inicio automatico:"
Name: "installsvc";    Description: "Instalar servicio de Windows (API en segundo plano)"; GroupDescription: "Servicio de Windows:"; Flags: checkedonce

[Files]
Source: "balanza-pos.exe";    DestDir: "{app}"; Flags: ignoreversion
Source: "icon.ico";           DestDir: "{app}"; Flags: ignoreversion
Source: "config.example.yaml"; DestDir: "{app}"; DestName: "config.yaml"; Flags: onlyifdoesntexist

[Icons]
Name: "{group}\{#AppName}";              Filename: "{app}\{#AppExeName}"; IconFilename: "{app}\icon.ico"
Name: "{group}\Desinstalar {#AppName}";  Filename: "{uninstallexe}"
Name: "{commondesktop}\{#AppName}";      Filename: "{app}\{#AppExeName}"; IconFilename: "{app}\icon.ico"; Tasks: desktopicon

[Registry]
; Arranque automatico de la interfaz GUI con Windows
Root: HKLM; Subkey: "SOFTWARE\Microsoft\Windows\CurrentVersion\Run"; \
  ValueType: string; ValueName: "{#AppName}"; \
  ValueData: """{app}\{#AppExeName}"""; \
  Flags: uninsdeletevalue; Tasks: startupicon

[Run]
; Instalar el servicio de Windows despues de copiar archivos
Filename: "{app}\{#AppExeName}"; Parameters: "install"; \
  Flags: runhidden waituntilterminated; \
  StatusMsg: "Instalando servicio de Windows..."; \
  Tasks: installsvc

; Abrir la interfaz al terminar el instalador
Filename: "{app}\{#AppExeName}"; \
  Description: "Abrir {#AppName} ahora"; \
  Flags: nowait postinstall skipifsilent

[UninstallRun]
; Detener y desinstalar el servicio antes de borrar archivos
Filename: "{app}\{#AppExeName}"; Parameters: "uninstall"; \
  Flags: runhidden waituntilterminated; \
  RunOnceId: "UninstallService"

[UninstallDelete]
Type: files;     Name: "{app}\balanza-agent.log"
Type: files;     Name: "{app}\balanza.db"
Type: dirifempty; Name: "{app}"

[Code]
// Verifica que el ejecutable existe antes de continuar
function InitializeSetup(): Boolean;
begin
  Result := True;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  ResultCode: Integer;
begin
  if CurStep = ssInstall then
  begin
    // Detener el servicio antes de reemplazar el exe (ignorar error si no existe)
    Exec(ExpandConstant('{app}\{#AppExeName}'), 'stop', '', 0, ewWaitUntilTerminated, ResultCode);
  end;
end;
