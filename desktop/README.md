# ReconX desktop

Thin Electron wrapper around the dashboard your controller VPS serves. The desktop app does **not** run a scanner — it just shows the dashboard, so you can check stats from any laptop.

## Architecture

```
[ Your Mac / Windows ]            [ Controller VPS ]
        │                              │
   reconx-desktop  ─── HTTPS ───►   nginx :80  →  Flask dashboard
   (Electron app)                   /api/* :5000
                                    /fleet-api :8787
```

The first launch asks for the controller's URL (e.g. `http://203.0.113.10`). The app remembers it in `~/Library/Application Support/ReconX/config.json` (macOS) or `%APPDATA%\ReconX\config.json` (Windows). Change it any time via the **ReconX → Change backend URL…** menu.

## Run from source

```bash
cd desktop
npm install
npm start
```

## Build installers

```bash
# macOS .dmg + .zip
npm run package:mac

# Windows .exe (NSIS installer + portable)
npm run package:win

# Both (requires macOS host for .dmg signing)
npm run package:all
```

Output lands in `desktop/dist/`. Builds are unsigned — for distribution you'll want to add an Apple Developer ID + Windows code-signing cert in `electron-builder`'s `mac.identity` / `win.certificateFile`.
