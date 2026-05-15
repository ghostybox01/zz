// ReconX desktop — thin Electron wrapper around the dashboard.
// On first launch, prompts for the controller URL (e.g. http://203.0.113.10).
// Persists settings in app.getPath('userData')/config.json.

const { app, BrowserWindow, Menu, dialog, ipcMain, shell } = require('electron')
const path = require('node:path')
const fs = require('node:fs')

const CONFIG_PATH = () => path.join(app.getPath('userData'), 'config.json')

function readConfig() {
  try { return JSON.parse(fs.readFileSync(CONFIG_PATH(), 'utf-8')) } catch { return {} }
}

function writeConfig(cfg) {
  try { fs.writeFileSync(CONFIG_PATH(), JSON.stringify(cfg, null, 2)) } catch (e) { console.error(e) }
}

async function promptForBackend(currentValue = '') {
  // Open the local setup view bundled in renderer/setup.html. It posts the URL
  // back via IPC; we resolve when received (or 5 min timeout).
  return new Promise((resolve) => {
    const w = new BrowserWindow({
      width: 460,
      height: 320,
      title: 'ReconX — connect',
      resizable: false,
      minimizable: false,
      maximizable: false,
      webPreferences: {
        preload: path.join(__dirname, 'preload.js'),
        contextIsolation: true,
      },
    })
    w.removeMenu()
    w.loadFile(path.join(__dirname, 'renderer', 'setup.html'), { query: { current: currentValue } })

    const onSubmit = (_e, url) => {
      ipcMain.removeListener('reconx:setup:submit', onSubmit)
      w.close()
      resolve((url || '').trim())
    }
    ipcMain.on('reconx:setup:submit', onSubmit)
    w.on('closed', () => { ipcMain.removeListener('reconx:setup:submit', onSubmit); resolve('') })
  })
}

async function ensureBackend() {
  let cfg = readConfig()
  if (cfg.backendUrl) return cfg.backendUrl
  const url = await promptForBackend('')
  if (!url) { app.quit(); return null }
  cfg.backendUrl = url
  writeConfig(cfg)
  return url
}

function createMainWindow(targetUrl) {
  const win = new BrowserWindow({
    width: 1440,
    height: 900,
    title: 'ReconX',
    backgroundColor: '#0a0c12',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
    },
  })

  const menu = Menu.buildFromTemplate([
    {
      label: 'ReconX',
      submenu: [
        { label: 'Reload', accelerator: 'CmdOrCtrl+R', click: () => win.reload() },
        { type: 'separator' },
        {
          label: 'Change backend URL…',
          click: async () => {
            const cur = readConfig().backendUrl || ''
            const next = await promptForBackend(cur)
            if (next) {
              const cfg = readConfig(); cfg.backendUrl = next; writeConfig(cfg)
              win.loadURL(next)
            }
          },
        },
        { type: 'separator' },
        { role: 'toggleDevTools' },
        { type: 'separator' },
        { role: 'quit' },
      ],
    },
    { role: 'editMenu' },
    { role: 'viewMenu' },
  ])
  Menu.setApplicationMenu(menu)

  win.loadURL(targetUrl).catch((err) => {
    dialog.showErrorBox('Cannot reach ReconX', `${targetUrl}\n\n${err.message}\n\nUse ReconX → Change backend URL to point at a different controller.`)
  })

  win.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })

  win.webContents.on('did-fail-load', (_e, _code, desc) => {
    dialog.showErrorBox('Failed to load dashboard', `${targetUrl}\n\n${desc}`)
  })
}

app.whenReady().then(async () => {
  const url = await ensureBackend()
  if (!url) return
  createMainWindow(url)

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createMainWindow(url)
  })
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})
