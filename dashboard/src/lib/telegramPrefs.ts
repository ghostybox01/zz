const LS_BOT = 'scan-cockpit-tg-bot'
const LS_CHAT = 'scan-cockpit-tg-chat'
const LS_NOTIFY = 'scan-cockpit-tg-notify'

export type TelegramPrefs = {
  botToken: string
  chatId: string
  notifyNewHits: boolean
}

export function loadTelegramPrefs(): TelegramPrefs {
  return {
    botToken: typeof localStorage !== 'undefined' ? localStorage.getItem(LS_BOT) ?? '' : '',
    chatId: typeof localStorage !== 'undefined' ? localStorage.getItem(LS_CHAT) ?? '' : '',
    notifyNewHits:
      typeof localStorage !== 'undefined' ? localStorage.getItem(LS_NOTIFY) === '1' : false,
  }
}

export function saveTelegramPrefs(p: TelegramPrefs) {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(LS_BOT, p.botToken)
  localStorage.setItem(LS_CHAT, p.chatId)
  localStorage.setItem(LS_NOTIFY, p.notifyNewHits ? '1' : '0')
}
