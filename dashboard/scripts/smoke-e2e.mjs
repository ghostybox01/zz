/** End-to-end smoke test for the ReconX dashboard.
 *  - Assumes a backend is already running on $RECONX_BACKEND (default localhost:5000)
 *  - Assumes vite preview is running on $RECONX_FRONT (default localhost:4173)
 *  - Drives every tab + primary control with Playwright
 *  - Captures console messages + page errors
 *  - Writes a JSON report to /tmp/reconx-smoke-report.json and prints a summary
 */
import { chromium } from 'playwright'
import { writeFileSync } from 'node:fs'

const URL = process.env.RECONX_FRONT ?? 'http://localhost:4173'

const log = (...a) => console.log('[smoke]', ...a)
const issues = []
const consoleLog = []

function addIssue(category, detail) {
  issues.push({ category, detail })
}

async function safeClick(page, selector, label) {
  log(`click → ${label}`)
  try {
    const handle = await page.locator(selector).first()
    if (await handle.count() === 0) {
      addIssue('missing', `${label}: selector "${selector}" not found`)
      return false
    }
    await handle.click({ timeout: 3000 })
    await page.waitForTimeout(350)
    return true
  } catch (e) {
    addIssue('click-fail', `${label}: ${e.message}`)
    return false
  }
}

const browser = await chromium.launch({ headless: true })
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
const page = await ctx.newPage()

page.on('console', (msg) => {
  const entry = { type: msg.type(), text: msg.text(), location: msg.location() }
  consoleLog.push(entry)
  if (entry.type === 'error' || entry.type === 'warning') {
    addIssue(`console-${entry.type}`, entry.text)
  }
})
page.on('pageerror', (err) => {
  addIssue('pageerror', `${err.name}: ${err.message}`)
})
page.on('requestfailed', (req) => {
  const url = req.url()
  // Only flag failures of our own /api or app assets — ignore fonts/cdns
  if (url.includes('/api') || url.startsWith(URL)) {
    addIssue('request-failed', `${req.method()} ${url} — ${req.failure()?.errorText}`)
  }
})

try {
  await page.goto(URL, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1500)

  // Tabs
  const tabs = ['overview', 'ravenx', 'warc', 'lists', 'fleet', 'findings', 'settings']
  for (const t of tabs) {
    await safeClick(page, `button#tab-${t}`, `tab:${t}`)
    await page.screenshot({ path: `/tmp/reconx-smoke-${t}.png`, fullPage: false }).catch(() => undefined)
    await page.waitForTimeout(450)
  }

  // Cracker workspace — session rail + panel
  await safeClick(page, `button#tab-ravenx`, 'tab:ravenx (back)')
  await page.waitForTimeout(400)
  const cwSessionCount = await page.locator('.cw-session').count()
  log(`cracker sessions: ${cwSessionCount}`)
  if (cwSessionCount > 0) {
    await safeClick(page, '.cw-rail__item', 'select crack session')
    await page.waitForTimeout(500)
    const viewStatsBtn = page.locator('button', { hasText: 'View Stats' })
    if (await viewStatsBtn.count() > 0) {
      await safeClick(page, 'button:has-text("View Stats")', 'open scan stats')
      await page.waitForTimeout(700)
      await safeClick(page, '.scan-detail__back', 'back from scan detail')
      await page.waitForTimeout(500)
    }
  }

  // WARC — toggle harvest + export button
  await safeClick(page, `button#tab-warc`, 'tab:warc')
  await page.waitForTimeout(400)
  await safeClick(page, 'button:has-text("Start harvest"), button:has-text("Stop harvest")', 'warc toggle harvest')
  await page.waitForTimeout(400)
  const exportBtn = page.locator('button:has-text("Export to list")')
  if (await exportBtn.count() > 0 && !(await exportBtn.isDisabled())) {
    await safeClick(page, 'button:has-text("Export to list")', 'warc export to list')
    await page.waitForTimeout(500)
  }

  // Lists tab — toggle a filter chip, expand a list preview
  await safeClick(page, `button#tab-lists`, 'tab:lists')
  await page.waitForTimeout(400)
  const filterCount = await page.locator('.lists-filter-row .scan-filter').count()
  for (let i = 0; i < filterCount; i++) {
    await safeClick(page, `.lists-filter-row .scan-filter >> nth=${i}`, `list-filter[${i}]`)
  }
  const previewCount = await page.locator('.tlist__preview summary').count()
  if (previewCount > 0) {
    await safeClick(page, '.tlist__preview summary', 'list preview expand')
  }
  // Click a VPS chip on the first list (if any)
  const chipCount = await page.locator('.tlist-chip').count()
  if (chipCount > 0) {
    await safeClick(page, '.tlist-chip:not([disabled])', 'list chip toggle')
  }

  // Fleet tab — click a fleet card start button when in live mode
  await safeClick(page, `button#tab-fleet`, 'tab:fleet')
  await page.waitForTimeout(800)
  const startBtnCount = await page.locator('.fnode .btn-glass--xs', { hasText: 'Start' }).count()
  if (startBtnCount > 0) {
    await safeClick(page, '.fnode .btn-glass--xs >> text=Start', 'fleet start button')
    await page.waitForTimeout(600)
  }

  // Findings — click first finding to drill in, back
  await safeClick(page, `button#tab-findings`, 'tab:findings')
  await page.waitForTimeout(500)
  const findingRows = await page.locator('.finding-row--clickable').count()
  if (findingRows > 0) {
    await safeClick(page, '.finding-row--clickable', 'open finding detail')
    await page.waitForTimeout(700)
    await safeClick(page, '.fdv-btn', 'back from finding detail')
  }

  // Settings — open every accordion
  await safeClick(page, `button#tab-settings`, 'tab:settings')
  await page.waitForTimeout(500)
  const accCount = await page.locator('.settings-acc > summary').count()
  for (let i = 0; i < accCount; i++) {
    await safeClick(page, `.settings-acc:nth-of-type(${i + 1}) > summary`, `accordion[${i}]`)
  }
} catch (e) {
  addIssue('runner', `${e.name}: ${e.message}`)
} finally {
  await browser.close()
}

const summary = {
  url: URL,
  issues,
  consoleLog: consoleLog.filter((c) => c.type !== 'log').slice(-40),
}
writeFileSync('/tmp/reconx-smoke-report.json', JSON.stringify(summary, null, 2))

const counts = issues.reduce((m, i) => { m[i.category] = (m[i.category] ?? 0) + 1; return m }, {})
console.log('\n══ smoke report ══')
console.log(`url:    ${URL}`)
console.log(`issues: ${issues.length}`)
for (const [k, v] of Object.entries(counts)) console.log(`  - ${k}: ${v}`)
if (issues.length === 0) {
  console.log('✓ clean')
} else {
  console.log('\nfirst 20 issues:')
  for (const i of issues.slice(0, 20)) console.log(`  [${i.category}] ${i.detail.slice(0, 200)}`)
  console.log('\nfull report → /tmp/reconx-smoke-report.json')
}
process.exit(issues.length === 0 ? 0 : 1)
