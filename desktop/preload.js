const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('reconx', {
  submitBackend: (url) => ipcRenderer.send('reconx:setup:submit', url),
})
