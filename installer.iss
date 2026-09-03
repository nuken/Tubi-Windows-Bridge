[Setup]
AppName=Tubi Bridge
AppVersion=1.0.0
DefaultDirName={autopf}\TubiBridge
DefaultGroupName=Tubi Bridge
OutputBaseFilename=TubiBridgeSetup
Compression=lzma
SolidCompression=yes
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64os
SetupIconFile=icon.ico

[Files]
Source: "tubi-bridge.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autodesktop}\Tubi Bridge Dashboard"; Filename: "http://localhost:7778"; IconFilename: "{app}\tubi-bridge.exe"

[Run]
; 1. Install and start Windows service
Filename: "{sys}\sc.exe"; Parameters: "create TubiBridge start= auto binPath= ""{app}\tubi-bridge.exe"""; Flags: runhidden
Filename: "{sys}\sc.exe"; Parameters: "start TubiBridge"; Flags: runhidden

; 2. Allow the binary itself through the firewall (covers port 7778 or any fallback port dynamically)
Filename: "netsh"; Parameters: "advfirewall firewall add rule name=""Tubi Bridge"" dir=in action=allow program=""{app}\tubi-bridge.exe"" enable=yes"; Flags: runhidden

; 3. Open browser on install finish
Filename: "http://localhost:7778"; Description: "Open Tubi Bridge Dashboard"; Flags: postinstall shellexec nowait

[UninstallRun]
Filename: "{sys}\sc.exe"; Parameters: "stop TubiBridge"; Flags: runhidden; RunOnceId: "StopTubiBridge"
Filename: "{sys}\sc.exe"; Parameters: "delete TubiBridge"; Flags: runhidden; RunOnceId: "DeleteTubiBridge"
Filename: "netsh"; Parameters: "advfirewall firewall delete rule name=""Tubi Bridge"""; Flags: runhidden; RunOnceId: "DeleteTubiFirewall"