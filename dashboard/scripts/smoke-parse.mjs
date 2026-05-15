// Smoke test: hit the fixture HTTP server and run the parser used by the dashboard.
// Not part of the build — handy for verifying the right-split logic + masking on real schemas.

import { readFile } from 'node:fs/promises'
import { register } from 'node:module'
import { pathToFileURL } from 'node:url'

// We can't trivially run TS from Node without a loader. Instead, this script
// re-implements the right-split parser inline and just exercises the schemas
// I care about. If the right-split behavior changes in src/lib/parseScanFiles.ts,
// keep them in sync (it's a 15-line function).

function splitRight(line, trailingFields, hasSourceUrl) {
  if (!hasSourceUrl) {
    const parts = line.split(':')
    return { url: '', parts: parts.slice(0, trailingFields) }
  }
  if (trailingFields <= 0) return { url: line, parts: [] }
  const idxs = []
  for (let i = line.length - 1; i >= 0 && idxs.length < trailingFields; i--) {
    if (line[i] === ':') idxs.push(i)
  }
  if (idxs.length < trailingFields) return { url: line, parts: [] }
  idxs.reverse()
  const url = line.slice(0, idxs[0])
  const parts = []
  for (let i = 0; i < idxs.length; i++) {
    const start = idxs[i] + 1
    const end = i + 1 < idxs.length ? idxs[i + 1] : line.length
    parts.push(line.slice(start, end))
  }
  return { url, parts }
}

const cases = [
  // [label, line, trailingFields, hasUrl, expectedUrl, expectedParts]
  ['aws_valid', 'AKIATEST1234EXAMPLE:secretsecret', 2, false, '', ['AKIATEST1234EXAMPLE', 'secretsecret']],
  ['github_simple', 'https://target.example.com/.env:ghp_demoToken', 1, true, 'https://target.example.com/.env', ['ghp_demoToken']],
  ['github_url_with_port', 'https://target.example.com:8443/.env:ghp_demoToken', 1, true, 'https://target.example.com:8443/.env', ['ghp_demoToken']],
  ['aws_creds_4', 'https://t.example.com/.env:AKIATEST:secretsecret:us-east-1', 3, true, 'https://t.example.com/.env', ['AKIATEST', 'secretsecret', 'us-east-1']],
  ['twilio_3', 'https://t.example.com/cfg.json:AC1234sid:auth_token_value', 2, true, 'https://t.example.com/cfg.json', ['AC1234sid', 'auth_token_value']],
  ['url_only_backup', 'https://t.example.com/backup.zip', 0, true, 'https://t.example.com/backup.zip', []],
]

let failed = 0
for (const [label, line, tf, hasUrl, expUrl, expParts] of cases) {
  const got = splitRight(line, tf, hasUrl)
  const ok = got.url === expUrl && JSON.stringify(got.parts) === JSON.stringify(expParts)
  if (!ok) {
    failed++
    console.log(`FAIL ${label}`)
    console.log('  line:', JSON.stringify(line))
    console.log('  exp :', { url: expUrl, parts: expParts })
    console.log('  got :', got)
  } else {
    console.log(`OK   ${label}`)
  }
}

// Also fetch the fixture HTTP endpoint to confirm round-trip.
const base = process.env.FIXTURE_BASE ?? 'http://localhost:8088/'
try {
  const r = await fetch(base + 'aws_valid.txt')
  const t = await r.text()
  console.log(`---\nHTTP ${r.status} ${base}aws_valid.txt`)
  console.log(t.trim())
  if (r.status !== 200) failed++
} catch (e) {
  console.log('---\nHTTP fetch failed:', e.message)
  failed++
}

process.exit(failed ? 1 : 0)
