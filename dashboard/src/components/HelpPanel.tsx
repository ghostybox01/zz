export function HelpPanel() {
  return (
    <div className="help-panel">
      <div className="help-panel__hero">
        <h1 className="help-panel__title">ReconX — Operator Guide</h1>
        <p className="help-panel__sub">
          A fast, self-hosted credential-intelligence platform. Scan targets for leaked API keys,
          validate them against live provider endpoints, and surface actionable results.
        </p>
      </div>

      <div className="help-panel__grid">

        {/* ── GETTING STARTED ─────────────────────────────── */}
        <section className="help-card">
          <h2 className="help-card__heading">Getting started</h2>
          <ol className="help-card__list help-card__list--ol">
            <li>Open <strong>Settings → Target list upload</strong> and paste or upload a list of hosts (one per line, <code>host:port</code> optional).</li>
            <li>Go to <strong>Settings → Cracker addons</strong> and enable the credential categories you care about.</li>
            <li>Go to <strong>Settings → Scanner configuration</strong> and review thread counts, timeouts, and depth.</li>
            <li>Switch to the <strong>Cracker</strong> tab and press <strong>Start scan</strong>.</li>
            <li>Watch live results in the <strong>Hits</strong> tab as valid keys are discovered.</li>
          </ol>
        </section>

        {/* ── ARCHITECTURE ────────────────────────────────── */}
        <section className="help-card">
          <h2 className="help-card__heading">How it works</h2>
          <ul className="help-card__list">
            <li><strong>Scanner binary</strong> — Go binary that crawls targets looking for secrets in source, env files, and config pages.</li>
            <li><strong>Detector modules</strong> — Per-provider pattern matchers that extract candidate keys and validate them against the provider's API.</li>
            <li><strong>Backend (Flask)</strong> — Exposes the REST API the dashboard calls; proxies scan results and stores them in R2/local storage.</li>
            <li><strong>Dashboard</strong> — This React console. All scan control, result browsing, and fleet management happen here.</li>
            <li><strong>Fleet</strong> — Optionally run multiple VPS nodes via SSH shards; the controller distributes target chunks across all nodes.</li>
          </ul>
        </section>

        {/* ── TABS ────────────────────────────────────────── */}
        <section className="help-card help-card--wide">
          <h2 className="help-card__heading">Dashboard tabs</h2>
          <table className="help-table">
            <thead>
              <tr><th>Tab</th><th>What it does</th></tr>
            </thead>
            <tbody>
              <tr><td><strong>Dashboard</strong></td><td>Live counters, CPU usage, and recent activity feed across all nodes.</td></tr>
              <tr><td><strong>Cracker</strong></td><td>Start/stop scans, pick targets and enabled addons, watch real-time progress.</td></tr>
              <tr><td><strong>WARC</strong></td><td>Harvest full HTTP archives (WARC files) of target pages for offline analysis.</td></tr>
              <tr><td><strong>Lists</strong></td><td>Manage saved target lists stored in Cloudflare R2. Create, import, and delete.</td></tr>
              <tr><td><strong>Fleet</strong></td><td>Enroll VPS nodes via SSH and dispatch scan shards across the fleet.</td></tr>
              <tr><td><strong>Hits</strong></td><td>Browse every validated credential finding with source URL and service info.</td></tr>
              <tr><td><strong>Stripe</strong></td><td>Read-only view of discovered Stripe keys — balance and account details, no charges.</td></tr>
              <tr><td><strong>Crypto</strong></td><td>Read-only view of crypto wallet keys found — balance lookups only.</td></tr>
              <tr><td><strong>Dorks</strong></td><td>Build and run Google/Shodan/FOFA dorks. Multi-select saved dorks for bulk runs; collect hosts into a list.</td></tr>
              <tr><td><strong>Logs</strong></td><td>Raw scanner logs streamed from the active node.</td></tr>
              <tr><td><strong>Settings</strong></td><td>All configuration: targets, addons, scanner flags, fleet bootstrap, Telegram, R2, schedules.</td></tr>
            </tbody>
          </table>
        </section>

        {/* ── ADDON CATEGORIES ────────────────────────────── */}
        <section className="help-card help-card--wide">
          <h2 className="help-card__heading">Cracker addon categories</h2>
          <table className="help-table">
            <thead>
              <tr><th>Category</th><th>Examples</th><th>Notes</th></tr>
            </thead>
            <tbody>
              <tr><td>AI Keys</td><td>OpenAI, Anthropic, Cohere, Mistral…</td><td>Validates against each provider's models/list endpoint.</td></tr>
              <tr><td>Cloud (AWS)</td><td>AWS, GCP, DigitalOcean, Cloudflare…</td><td>AWS keys checked for IAM, S3, SES access.</td></tr>
              <tr><td>Email APIs</td><td>SendGrid, Mailgun, Brevo, Postmark, SparkPost…</td><td>Validates via /account or /messages endpoint.</td></tr>
              <tr><td>SMTP senders</td><td>SMTP credentials for popular relay services.</td><td>Attempts STARTTLS handshake to confirm login.</td></tr>
              <tr><td>Payment</td><td>Stripe, PayPal</td><td>Read-only balance/account check only — no charges.</td></tr>
              <tr><td>SMS providers</td><td>Twilio, Nexmo, MessageBird, Plivo…</td><td>Balance lookup and account confirmation.</td></tr>
              <tr><td>Version control</td><td>GitHub PAT, GitLab PAT</td><td>Lists repos to confirm scope.</td></tr>
              <tr><td>Developer tools</td><td>Heroku, Datadog, Sentry, NPM…</td><td>Account/org endpoint validation.</td></tr>
              <tr><td>Crypto wallets</td><td>Bitcoin, Ethereum, Solana private keys</td><td>Derives public address and fetches on-chain balance. Enable in Settings first.</td></tr>
            </tbody>
          </table>
        </section>

        {/* ── DORKS ───────────────────────────────────────── */}
        <section className="help-card">
          <h2 className="help-card__heading">Dork hunter</h2>
          <ul className="help-card__list">
            <li>Type a dork query and choose <strong>Google</strong>, <strong>Shodan</strong>, or <strong>FOFA</strong> as the platform.</li>
            <li>Press <strong>Run</strong> to execute the dork. Shodan/FOFA results are fetched via the backend; Google opens a browser tab.</li>
            <li>Save frequently used dorks for later recall.</li>
            <li><strong>Multi-select:</strong> tick the checkboxes on saved dorks (or "All"), then press <strong>Run selected</strong>. Results are deduplicated and you can save the collected hosts directly as a new target list.</li>
          </ul>
        </section>

        {/* ── FLEET ───────────────────────────────────────── */}
        <section className="help-card">
          <h2 className="help-card__heading">Fleet (multi-VPS)</h2>
          <ul className="help-card__list">
            <li>Paste or import VPS SSH credentials in <strong>Settings → Fleet bootstrap</strong>.</li>
            <li>The controller splits your target list into equal chunks and dispatches them via SSH.</li>
            <li>Each shard's progress appears in the Fleet tab. Results flow back to the same Hits / Findings view.</li>
            <li>Install ReconX on a fresh VPS with the provided <code>deploy.py</code> script (<code>python deploy.py &lt;host&gt;</code>).</li>
          </ul>
        </section>

        {/* ── UPDATES ─────────────────────────────────────── */}
        <section className="help-card">
          <h2 className="help-card__heading">Keeping up to date</h2>
          <ul className="help-card__list">
            <li>The <strong>Updates</strong> section in Settings shows current build SHA vs latest.</li>
            <li>On the VPS, run <code>sudo reconx-update</code> to pull and rebuild the latest version.</li>
            <li>The update helper fetches the correct branch automatically.</li>
          </ul>
        </section>

        {/* ── R2 ──────────────────────────────────────────── */}
        <section className="help-card">
          <h2 className="help-card__heading">Cloudflare R2 storage</h2>
          <ul className="help-card__list">
            <li>Configure a Cloudflare R2 bucket in <strong>Settings → R2 storage</strong>.</li>
            <li>Target lists are uploaded to R2 and synced to the scanner at scan start.</li>
            <li>Results files (<code>valid_*.txt</code>) are also uploaded to R2 after each run for persistent storage.</li>
          </ul>
        </section>

        {/* ── TELEGRAM ────────────────────────────────────── */}
        <section className="help-card">
          <h2 className="help-card__heading">Telegram notifications</h2>
          <ul className="help-card__list">
            <li>Add your bot token and chat ID in <strong>Settings → Notifications → Telegram</strong>.</li>
            <li>Every validated credential triggers a rich Telegram message with key, provider, and source URL.</li>
            <li>Use the notification filters to suppress categories you don't want alerted.</li>
          </ul>
        </section>

      </div>
    </div>
  )
}
