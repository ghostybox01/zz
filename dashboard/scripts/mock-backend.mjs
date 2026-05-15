/** Minimal mock of the Raven Flask backend (app.py) for smoke testing.
 *  Implements /api/stats, /api/vps/available, /api/vps/status, /api/vps/servers,
 *  /api/vps/server/<ip>/start|stop|restart, /api/vps/upload-targets, /api/vps/deploy.
 *  Socket.io is NOT mocked (the dashboard falls back to REST polling when the socket fails).
 *  Run: node scripts/mock-backend.mjs
 */
import http from 'node:http'
import { URL } from 'node:url'

const PORT = Number(process.env.PORT ?? 5000)

const findings = [
  ['AWS',      'AKIATEST0000EXAMPLE',                              'https://edge.test/.env',       new Date(Date.now() - 4 * 60_000).toISOString(),    'us-east-1'],
  ['Stripe',   'sk_test_REDACTED_MOCK_PLACEHOLDER',                'https://pay.test/.env',        new Date(Date.now() - 11 * 60_000).toISOString(),   'mode=live'],
  ['SendGrid', 'SG.TEST.tokentokentokentokentokenN4kQ',            'https://mail.test/api/.env',   new Date(Date.now() - 18 * 60_000).toISOString(),   'plan=Pro 100K'],
  ['Mailgun',  'key-TESTxxxxxxxxxxxxxxxxxxxxxxxxxxxx',             'https://newsletter.test/.env', new Date(Date.now() - 25 * 60_000).toISOString(),   'domain=test'],
  ['Twilio',   'AC0TEST00000000000000000000000000',                'https://sms.test/.env',        new Date(Date.now() - 33 * 60_000).toISOString(),   'numbers=4'],
  ['SMTP',     'user@host.test:s3cret',                            'https://mail.test/wp.bak',     new Date(Date.now() - 47 * 60_000).toISOString(),   'authMethod=LOGIN'],
]

let mockPaths = { present: false, lines: 0, source: 'builtin' }
const mockTelegram = { has_token: false, token_tail: '', chat_id: '' }

const scannerCfg = {
  scanning_features: { aws_main_scan: true, github_token_deep_scan: true, smtp_credentials_scan: true },
  aws_checks: { ses_quota_check: true, sns_limit_check: true, fargate_limit_check: true, federation_console_url: true },
  api_validation: {
    openai: true, anthropic: true, stripe: true, gcp_api_key: false,
    sendgrid: true, mailgun: true, twilio: true, nexmo: true,
    telnyx: true, messagebird: false, github: true,
  },
  features: { brevo: true, xsmtp: true, mandrill: true, mailersend: true, new_mailgun: true },
  exploit_methods: { react2shell: true, bypass_waf: true, bypass_middleware: true, lfi: true, xxe: true, ssrf: true },
}

const servers = [
  {
    ip: '198.51.100.10', status: 'RUNNING', scanned: 12_500, targets: 50_000, hits: 84, speed: 38.4,
    uptime: '6h 12m', batch_info: 'batch 3/12 — processing', batches_done: 2, batches_total: 12,
    current_batch_progress: 42, last_update: new Date().toISOString(), error: null,
  },
  {
    ip: '198.51.100.11', status: 'RUNNING', scanned: 18_204, targets: 50_000, hits: 91, speed: 41.1,
    uptime: '6h 12m', batch_info: 'batch 4/12 — processing', batches_done: 3, batches_total: 12,
    current_batch_progress: 18, last_update: new Date().toISOString(), error: null,
  },
  {
    ip: '198.51.100.12', status: 'OFFLINE', scanned: 0, targets: 50_000, hits: 0, speed: 0,
    uptime: '—', batch_info: '-', batches_done: 0, batches_total: 12,
    current_batch_progress: 0, last_update: new Date().toISOString(), error: 'SSH unreachable',
  },
]

function send(res, status, body) {
  res.writeHead(status, {
    'Content-Type': 'application/json',
    'Access-Control-Allow-Origin': '*',
    'Access-Control-Allow-Headers': 'Content-Type, Authorization',
    'Access-Control-Allow-Methods': 'GET, POST, PUT, OPTIONS',
  })
  res.end(JSON.stringify(body))
}

async function readBody(req) {
  let raw = ''
  for await (const chunk of req) raw += chunk
  return raw
}

const server = http.createServer(async (req, res) => {
  if (req.method === 'OPTIONS') {
    res.writeHead(204, {
      'Access-Control-Allow-Origin': '*',
      'Access-Control-Allow-Headers': 'Content-Type, Authorization',
      'Access-Control-Allow-Methods': 'GET, POST, PUT, OPTIONS',
    })
    res.end()
    return
  }

  const url = new URL(req.url ?? '/', `http://${req.headers.host}`)
  const p = url.pathname

  // Socket.io polling — let it 404 so client falls back to pure REST.
  if (p.startsWith('/socket.io/')) {
    res.writeHead(404)
    res.end()
    return
  }

  if (p === '/api/stats' && req.method === 'GET') {
    return send(res, 200, {
      total_urls: 32_704,
      total_hits: 6,
      total_valid: 6,
      smtp_servers: 1,
      type_counts: { AWS: 1, Stripe: 1, SendGrid: 1, Mailgun: 1, Twilio: 1, SMTP: 1 },
      recent_findings: findings,
      last_update: new Date().toISOString(),
      progress_current: 32_704,
      progress_total: 100_000,
      progress_percent: 32.7,
      scan_rate: 18.4,
    })
  }

  if (p === '/api/clear' && req.method === 'POST') {
    return send(res, 200, { success: true, message: 'Mock cleared' })
  }

  if (p === '/api/vps/available' && req.method === 'GET') {
    return send(res, 200, { available: true })
  }

  if (p === '/api/scanner-config') {
    if (req.method === 'GET') return send(res, 200, scannerCfg)
    if (req.method === 'POST') {
      const patch = JSON.parse((await readBody(req)) || '{}')
      for (const section of Object.keys(scannerCfg)) {
        if (patch[section] && typeof patch[section] === 'object') {
          scannerCfg[section] = { ...scannerCfg[section], ...patch[section] }
        }
      }
      return send(res, 200, scannerCfg)
    }
  }

  if (p === '/api/fleet/bulk-creds' && req.method === 'POST') {
    const raw = await readBody(req)
    let lines = []
    try {
      const j = JSON.parse(raw)
      if (j && typeof j.text === 'string') lines = j.text.split('\n')
      else if (j && Array.isArray(j.creds)) lines = j.creds.map((c) => c.host || '')
    } catch { lines = raw.split('\n') }
    const parsed = lines.map((l) => (l || '').trim()).filter((l) => l && !l.startsWith('#'))
    const results = parsed.map((l, i) => {
      const at = l.indexOf('@')
      const userHost = at >= 0 ? l : `root@${l}`
      const [user, hostPart] = userHost.split('@')
      const [host, portStr] = hostPart.split(':')
      const ok = i % 3 !== 1
      return { host, port: Number(portStr) || 22, user, ok, message: ok ? 'connected (mock)' : 'auth failed (mock)' }
    })
    return send(res, 200, {
      total: results.length,
      ok: results.filter((r) => r.ok).length,
      failed: results.filter((r) => !r.ok).length,
      results,
      added_to_roster: results.filter((r) => r.ok).length,
    })
  }

  if (p === '/api/telegram') {
    if (req.method === 'GET') return send(res, 200, mockTelegram)
    if (req.method === 'POST') {
      const body = JSON.parse((await readBody(req)) || '{}')
      if (typeof body.bot_token === 'string') {
        mockTelegram.has_token = !!body.bot_token
        mockTelegram.token_tail = body.bot_token.length > 4 ? `…${body.bot_token.slice(-4)}` : ''
      }
      if (typeof body.chat_id === 'string') mockTelegram.chat_id = body.chat_id
      return send(res, 200, mockTelegram)
    }
  }

  if (p === '/api/telegram/test' && req.method === 'POST') {
    await readBody(req)
    if (!mockTelegram.has_token || !mockTelegram.chat_id) {
      return send(res, 400, { success: false, error: 'Telegram not configured (mock).' })
    }
    return send(res, 200, { success: true })
  }

  if (p === '/api/scanner-paths') {
    if (req.method === 'GET') return send(res, 200, mockPaths)
    if (req.method === 'DELETE') { mockPaths = { present: false, lines: 0, source: 'builtin' }; return send(res, 200, mockPaths) }
    if (req.method === 'POST') {
      const raw = await readBody(req)
      // Estimate line count by counting newlines (covers raw text + multipart prefix/suffix imprecisely)
      const lines = Math.max(1, (raw.match(/\n/g) ?? []).length - 2)
      mockPaths = { present: true, lines, source: 'paths.txt' }
      return send(res, 200, mockPaths)
    }
  }

  if (p === '/api/vps/config') {
    if (req.method === 'GET') return send(res, 200, { ssh_key_path: '/root/ssh/1', remote_user: 'root', work_dir: '/root/python_job', batch_size: 100_000, target_file: 'targets.txt', ssh_timeout: 5 })
    if (req.method === 'POST') return send(res, 200, { success: true, config: {} })
  }

  if (p === '/api/vps/servers') {
    if (req.method === 'GET') return send(res, 200, { servers: servers.map((s) => s.ip) })
    if (req.method === 'POST') {
      const body = JSON.parse((await readBody(req)) || '{}')
      return send(res, 200, { success: true, count: body.servers?.length ?? 0 })
    }
    if (req.method === 'PUT') {
      const body = JSON.parse((await readBody(req)) || '{}')
      return send(res, 200, { success: true, servers: [...servers.map((s) => s.ip), body.ip].filter(Boolean) })
    }
  }

  if (p === '/api/vps/status' && req.method === 'GET') {
    return send(res, 200, {
      servers,
      stats: {
        total_servers: servers.length,
        online: servers.filter((s) => s.status === 'RUNNING').length,
        offline: servers.filter((s) => s.status !== 'RUNNING').length,
        total_scanned: servers.reduce((a, s) => a + s.scanned, 0),
        total_hits: servers.reduce((a, s) => a + s.hits, 0),
        total_speed: servers.reduce((a, s) => a + s.speed, 0),
      },
    })
  }

  const perServer = p.match(/^\/api\/vps\/server\/([^/]+)\/(start|stop|restart|test|diagnose|logs|fix|deploy|collect|status)$/)
  if (perServer) {
    const action = perServer[2]
    const ip = decodeURIComponent(perServer[1])
    if (action === 'status' && req.method === 'GET') {
      const s = servers.find((x) => x.ip === ip)
      return send(res, 200, s ?? { ip, status: 'UNKNOWN' })
    }
    if (action === 'logs') return send(res, 200, { success: true, logs: [`[mock] tail for ${ip}`, '[INFO] Scanner running', `[OK] ${ip} processed 12500 targets`] })
    return send(res, 200, { success: true, message: `mock ${action} ${ip}` })
  }

  if (p.startsWith('/api/vps/start-all') || p.startsWith('/api/vps/stop-all') || p.startsWith('/api/vps/restart-all') || p.startsWith('/api/vps/deploy-all') || p.startsWith('/api/vps/collect-all') || p.startsWith('/api/vps/test-connections') || p.startsWith('/api/vps/test-ssh')) {
    return send(res, 200, { success: true, message: `mock ${p}` })
  }

  if (p === '/api/vps/upload-targets' && req.method === 'POST') {
    // Read body but don't parse multipart fully — just report a plausible response
    await readBody(req)
    return send(res, 200, { success: true, filename: 'targets.txt', targets: 12_500 })
  }

  if (p === '/api/vps/prepare-deploy' && req.method === 'POST') {
    await readBody(req)
    return send(res, 200, { success: true, target_file: 'targets.txt', total_targets: 12_500, batch_size: 4_166, servers: servers.map((s) => s.ip) })
  }

  if (p === '/api/vps/deploy' && req.method === 'POST') {
    await readBody(req)
    return send(res, 200, { success: true, message: 'Deployed across rostered fleet (mock)' })
  }

  if (p === '/api/vps/list-files' && req.method === 'GET') {
    return send(res, 200, { files: [{ name: 'targets.txt', path: 'targets.txt', size: 124_000, lines: 12_500 }] })
  }

  if (p === '/api/vps/select-file' && req.method === 'POST') {
    await readBody(req)
    return send(res, 200, { success: true, path: 'targets.txt', filename: 'targets.txt', targets: 12_500 })
  }

  res.writeHead(404, { 'Content-Type': 'application/json', 'Access-Control-Allow-Origin': '*' })
  res.end(JSON.stringify({ error: `mock backend: no route for ${req.method} ${p}` }))
})

server.listen(PORT, () => {
  console.log(`[mock-backend] listening on http://localhost:${PORT}`)
})
