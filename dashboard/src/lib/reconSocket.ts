/** Singleton Socket.IO client for the Raven backend.
 *  Used by the stats / fleet hooks to subscribe to `stats_update` and `vps_update` events.
 */
import { io, type Socket } from 'socket.io-client'

let _socket: Socket | null = null

export function getReconSocket(): Socket {
  if (_socket) return _socket
  _socket = io({
    transports: ['websocket', 'polling'],
    autoConnect: true,
    reconnection: true,
    reconnectionAttempts: Infinity,
    reconnectionDelay: 1000,
    reconnectionDelayMax: 5000,
  })
  return _socket
}

export function disconnectReconSocket(): void {
  if (_socket) {
    _socket.disconnect()
    _socket = null
  }
}
