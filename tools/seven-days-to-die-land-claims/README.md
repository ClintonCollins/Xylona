# Xylona Land Claims

This mod adds the authenticated `GET /api/getlandclaims` endpoint consumed by
Xylona's 7 Days to Die map. The 2.6 and 3.x projects share one source file but
reference the WebAPI from different game assemblies.

Build both targets with dedicated-server managed assemblies:

```powershell
.\build.ps1 -V26ServerRoot C:\servers\7dtd-v2.6 -V3ServerRoot C:\servers\7dtd-v3
```

Each root must have the standard dedicated-server layout. The 2.6 server must
include `Mods/TFP_WebServer/WebServer.dll`. Build output is written directly
to the packaged asset directories used by the controller.
