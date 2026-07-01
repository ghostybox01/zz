import { useEffect, useRef, useState } from 'react'
import { warc, r2, type WarcStatus, type R2Config, type R2Object } from '../lib/reconApi'
import { labelForHost } from '../lib/labelForHost'
import type { VpsNode } from '../types'

const POLL_MS = 3000

// ── Domain category presets ──────────────────────────────────────────────────
// Each entry expands to a comma-separated domain list that gets merged into the
// crt.sh domains field. Toggling a preset off removes exactly those domains.
const DOMAIN_PRESETS: { id: string; label: string; domains: readonly string[] }[] = [
  {
    id: 'payments',
    label: 'Payments',
    domains: [
      'stripe.com', 'square.com', 'bill.com', 'braintreepayments.com', 'adyen.com',
      'klarna.com', 'checkout.com', 'recurly.com', 'chargebee.com', 'paddle.com',
      'mollie.com', 'razorpay.com', 'payoneer.com', 'sumup.com', 'zettle.com',
      'gocardless.com', 'spreedly.com', 'mangopay.com', 'zuora.com', 'chargify.com',
      'affirm.com', 'afterpay.com', 'sezzle.com', 'zip.co', 'paypal.com',
      'braintree.com', '2checkout.com', 'worldpay.com', 'payu.com',
      'cybersource.com', 'authorize.net', 'nuvei.com', 'payfirma.com',
    ],
  },
  {
    id: 'ecommerce',
    label: 'Ecommerce',
    domains: [
      'shopify.com', 'bigcommerce.com', 'woocommerce.com', 'magento.com',
      'prestashop.com', 'opencart.com', 'squarespace.com', 'wix.com',
      'ecwid.com', 'sellfy.com', 'gumroad.com', 'samcart.com', 'thrivecart.com',
      'payhip.com', 'lemonsqueezy.com', 'recharge.com', 'bold.com',
      'snipcart.com', 'salesforce.com', 'commercetools.com', 'crystallize.com',
      'medusajs.com', 'fabric.inc', 'centra.com', 'spryker.com',
    ],
  },
  {
    id: 'email-api',
    label: 'Email APIs',
    domains: [
      'sendgrid.com', 'brevo.com', 'mailgun.com', 'sparkpost.com', 'postmarkapp.com',
      'elasticemail.com', 'smtp2go.com', 'mailjet.com', 'mandrill.com',
      'campaignmonitor.com', 'constantcontact.com', 'klaviyo.com', 'drip.com',
      'activecampaign.com', 'mailchimp.com', 'aweber.com', 'getresponse.com',
      'convertkit.com', 'mailerlite.com', 'moosend.com', 'omnisend.com',
      'sendpulse.com', 'mailtrap.io', 'emarsys.com', 'dotdigital.com',
      'sailthru.com', 'maropost.com', 'iterable.com', 'blastable.com',
      'mailersend.com', 'socketlabs.com', 'pepipost.com', 'dyn.com',
    ],
  },
  {
    id: 'cloud',
    label: 'Cloud / Hosting',
    domains: [
      'amazonaws.com', 'googlecloud.com', 'azure.com', 'digitalocean.com',
      'linode.com', 'vultr.com', 'hetzner.com', 'ovh.com', 'rackspace.com',
      'heroku.com', 'render.com', 'fly.io', 'netlify.com', 'vercel.com',
      'railway.app', 'upcloud.com', 'contabo.com', 'ionos.com', 'scaleway.com',
      'exoscale.com', 'civo.com', 'cloudways.com', 'kinsta.com', 'wpengine.com',
      'siteground.com', 'dreamhost.com', 'bluehost.com', 'godaddy.com',
    ],
  },
  {
    id: 'auth',
    label: 'Auth / SSO',
    domains: [
      'auth0.com', 'okta.com', 'onelogin.com', 'ping.com', 'jumpcloud.com',
      'fusionauth.io', 'stytch.com', 'clerk.com', 'duosecurity.com',
      'cyberark.com', 'sailpoint.com', 'saviynt.com', 'beyondtrust.com',
      'lastpass.com', '1password.com', 'bitwarden.com', 'dashlane.com',
      'hashicorp.com', 'workos.com', 'logto.io', 'ory.sh', 'supertokens.com',
    ],
  },
  {
    id: 'sms',
    label: 'SMS / Voice',
    domains: [
      'twilio.com', 'vonage.com', 'nexmo.com', 'messagebird.com', 'plivo.com',
      'sinch.com', 'bandwidth.com', 'telnyx.com', 'ringcentral.com', 'infobip.com',
      'clickatell.com', 'textmagic.com', 'bulksms.com', 'smsapi.com',
      'msg91.com', 'kaleyra.com', 'routemobile.com', 'unifonic.com',
      'telesign.com', 'mgage.com', 'zip.sms.com', 'textbelt.com',
      'sms77.io', 'seven.io', 'heymarket.com', 'podium.com',
    ],
  },
  {
    id: 'crm',
    label: 'CRM / Sales',
    domains: [
      'salesforce.com', 'hubspot.com', 'marketo.com', 'pardot.com', 'eloqua.com',
      'zendesk.com', 'intercom.com', 'freshdesk.com', 'drift.com', 'zoho.com',
      'pipedrive.com', 'freshworks.com', 'insightly.com', 'copper.com',
      'salesloft.com', 'outreach.io', 'apollo.io', 'lemlist.com', 'lusha.com',
      'zoominfo.com', 'gong.io', 'chorus.ai', 'close.com', 'monday.com',
      'notion.so', 'attio.com', 'affinity.co', 'highlevel.com',
    ],
  },
  {
    id: 'analytics',
    label: 'Analytics',
    domains: [
      'mixpanel.com', 'segment.com', 'amplitude.com', 'heap.io', 'hotjar.com',
      'fullstory.com', 'logrocket.com', 'datadog.com', 'newrelic.com',
      'splunk.com', 'elastic.co', 'sumologic.com', 'dynatrace.com',
      'appdynamics.com', 'rollbar.com', 'sentry.io', 'bugsnag.com',
      'airbrake.io', 'raygun.com', 'logz.io', 'papertrail.com', 'posthog.com',
      'statsig.com', 'launchdarkly.com', 'growthbook.io', 'flagsmith.com',
      'pendo.io', 'appcues.com', 'userpilot.com', 'customerio.com',
    ],
  },
  {
    id: 'devops',
    label: 'DevOps / CI-CD',
    domains: [
      'github.com', 'gitlab.com', 'bitbucket.com', 'circleci.com',
      'travisci.com', 'semaphoreapp.com', 'buildkite.com', 'drone.io',
      'sonarcloud.io', 'snyk.io', 'codeclimate.com', 'codecov.io',
      'jfrog.com', 'sonatype.com', 'docker.com', 'hashicorp.com',
      'terraform.io', 'ansible.com', 'puppet.com', 'chef.io',
      'pagerduty.com', 'opsgenie.com', 'victorops.com', 'statuspage.io',
      'atlassian.com', 'linear.app', 'shortcut.com',
    ],
  },
  {
    id: 'storage',
    label: 'DB / Storage',
    domains: [
      'mongodb.com', 'supabase.com', 'planetscale.com', 'cockroachlabs.com',
      'fauna.com', 'redis.com', 'neon.tech', 'xata.io', 'turso.tech',
      'convex.dev', 'appwrite.io', 'arangodb.com', 'couchbase.com',
      'snowflake.com', 'databricks.com', 'firebolt.io', 'singlestore.com',
      'tidbcloud.com', 'yugabyte.com', 'influxdata.com', 'timescale.com',
      'hasura.io', 'prisma.io', 'edgedb.com', 'deno.com',
      'upstash.com', 'momento.io', 'fly.io',
    ],
  },
  {
    id: 'crypto',
    label: 'Crypto / Fintech',
    domains: [
      'coinbase.com', 'binance.com', 'kraken.com', 'blockchain.com', 'bitpay.com',
      'coinpayments.net', 'circle.com', 'paxos.com', 'fireblocks.com',
      'chainalysis.com', 'elliptic.co', 'anchorage.com', 'gemini.com',
      'bitfinex.com', 'bitstamp.com', 'crypto.com', 'nexo.com',
      'alchemy.com', 'infura.io', 'moralis.io', 'quicknode.com',
      'thegraph.com', 'blockdaemon.com', 'copper.co', 'bitgo.com',
      'taxbit.com', 'koinly.com', 'cryptotaxcalculator.io',
    ],
  },
  {
    id: 'esign',
    label: 'e-Sign / Forms',
    domains: [
      'docusign.com', 'hellosign.com', 'pandadoc.com', 'signnow.com',
      'adobe.com', 'signaturit.com', 'signeasy.com', 'eversign.com',
      'contractsafe.com', 'formstack.com', 'jotform.com', 'typeform.com',
      'docsketch.com', 'signwell.com', 'zoho.com', 'clio.com',
      'ironclad.com', 'docupace.com', 'legalesign.com',
    ],
  },
  {
    id: 'cdn',
    label: 'CDN / Media',
    domains: [
      'cloudflare.com', 'fastly.com', 'bunny.net', 'cdn77.com', 'keycdn.com',
      'stackpath.com', 'imgix.com', 'cloudinary.com', 'imagekit.io',
      'filestack.com', 'uploadcare.com', 'transloadit.com', 'bytescale.com',
      'mux.com', 'jwplayer.com', 'kaltura.com', 'vimeo.com', 'wistia.com',
      'brightcove.com', 'bitmovin.com', 'apivideo.com', 'encoding.com',
      'api.video', 'loom.com', 'bynder.com', 'canto.com',
    ],
  },
  {
    id: 'social',
    label: 'Social / OAuth',
    domains: [
      'facebook.com', 'twitter.com', 'x.com', 'google.com', 'linkedin.com',
      'github.com', 'instagram.com', 'discord.com', 'slack.com',
      'microsoft.com', 'apple.com', 'tiktok.com', 'pinterest.com',
      'reddit.com', 'spotify.com', 'amazon.com', 'paypal.com',
      'twitch.tv', 'snapchat.com', 'telegram.org',
    ],
  },
  {
    id: 'shipping',
    label: 'Shipping / Logistics',
    domains: [
      'shippo.com', 'shipbob.com', 'easypost.com', 'fedex.com', 'ups.com',
      'dhl.com', 'shipwire.com', 'whiplash.com', 'shipmonk.com',
      'stamps.com', 'pirateship.com', 'shipstation.com', 'shiphero.com',
      'ware2go.com', 'flexport.com', 'freightos.com', 'project44.com',
      'narvar.com', 'aftership.com', 'parcelhub.co.uk',
    ],
  },
  {
    id: 'accounting',
    label: 'Accounting / Tax',
    domains: [
      'quickbooks.com', 'intuit.com', 'xero.com', 'freshbooks.com', 'wave.com',
      'netsuite.com', 'sage.com', 'zoho.com', 'freeagent.com', 'clearbooks.co.uk',
      'taxjar.com', 'avalara.com', 'taxify.eu', 'sovos.com', 'vertex.com',
      'bill.com', 'divvy.co', 'ramp.com', 'brex.com', 'expensify.com',
      'concur.com', 'sap.com', 'oracle.com',
    ],
  },
  {
    id: 'maps',
    label: 'Maps / Geo',
    domains: [
      'google.com', 'mapbox.com', 'here.com', 'tomtom.com',
      'geocodio.com', 'opencagedata.com', 'ipstack.com', 'positionstack.com',
      'locationiq.com', 'geoapify.com', 'radar.io', 'foursquare.com',
      'smartystreets.com', 'lob.com', 'melissa.com', 'precisely.com',
      'loqate.com', 'geolocation.io', 'ipdata.co', 'ipgeolocation.io',
    ],
  },
  {
    id: 'support',
    label: 'Support / Chat',
    domains: [
      'zendesk.com', 'freshdesk.com', 'intercom.com', 'helpscout.com',
      'livechat.com', 'drift.com', 'crisp.chat', 'tawk.to', 'tidio.com',
      'olark.com', 'gorgias.com', 'kustomer.com', 'reamaze.com',
      'groovehq.com', 'kayako.com', 'happyfox.com', 'dixa.com',
      'gladly.com', 'sprinklr.com', 'freshchat.com', 'supportbee.com',
    ],
  },
]

/** Optional toast plumbing. WarcPanel renders standalone in any context, but
 * when the host page passes a notifier we surface Stop/Export progress and
 * completion through it instead of repurposing the inline `error` slot. */
export type WarcPanelToast = (title: string, message: string, kind: 'info' | 'error') => void

type Props = {
  notify?: WarcPanelToast
  /** Live fleet roster — used to resolve worker IPs in the "Run on" dropdown
   * to their operator-facing labels (`label (ip)`). Optional so the panel
   * still renders in isolation (tests, standalone embed); when absent, hosts
   * are shown raw. */
  fleet?: readonly VpsNode[]
}

export function WarcPanel({ notify, fleet = [] }: Props = {}) {
  const [status, setStatus] = useState<WarcStatus | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [maxDomains, setMaxDomains] = useState(10000)
  const [extractWorkers, setExtractWorkers] = useState(50)
  const [testWorkers, setTestWorkers] = useState(25)
  const [snapshots, setSnapshots] = useState(0)
  const [runOn, setRunOn] = useState<string>('controller')
  const [selectedNodes, setSelectedNodes] = useState<string[]>(['controller'])
  const [hosts, setHosts] = useState<string[]>(['controller'])
  // Producer source toggles — default mirrors the legacy CC-only flow.
  // Either may be true; both true runs them concurrently into the same
  // dedupe map on the worker.
  const [sourceCC, setSourceCC] = useState(true)
  const [sourceCrtSh, setSourceCrtSh] = useState(false)
  // crt.sh pivot inputs — comma-separated, free-text. Only meaningful
  // when sourceCrtSh is true; the form gates them visually.
  const [crtTld, setCrtTld] = useState('')
  const [crtDomain, setCrtDomain] = useState('')
  // Opt-in apex filter (drops FQDNs equal to their own eTLD+1).
  const [subdomainOnly, setSubdomainOnly] = useState(false)
  // Active preset IDs — drives the chip highlight state and toggle logic.
  const [activePresets, setActivePresets] = useState<ReadonlySet<string>>(new Set())
  // R2 health, refreshed alongside warc/status. Surfaces the same pill
  // R2Settings shows so the operator can see at the export site whether
  // a click on "Export to R2" will actually land.
  const [r2State, setR2State] = useState<R2Config['state']>('unknown')
  const [r2LastError, setR2LastError] = useState<string | null>(null)
  // R2 exports list — populated on demand (mount + after each export +
  // after each delete). Lets the operator see what landed and prune
  // duplicates without leaving the cockpit.
  const [r2Objects, setR2Objects] = useState<R2Object[]>([])
  const [r2Listing, setR2Listing] = useState(false)
  const [r2Merging, setR2Merging] = useState(false)
  const [r2MergeResult, setR2MergeResult] = useState<{ key: string; total: number } | null>(null)
  const [r2MergeError, setR2MergeError] = useState<string | null>(null)
  const [sysMem, setSysMem] = useState<{ total_mb: number; available_mb: number } | null>(null)
  const pollTimer = useRef<number | null>(null)

  function suggestWorkers(totalMb: number): { extract: number; test: number } {
    // Reserve 400 MB for OS + gunicorn + nginx, use 80% of the rest.
    // Each active extract worker needs ~8 MB (1 MB scanner buffer + HTTP/gzip overhead);
    // each test worker ~2 MB. Ratio kept at 2:1 (extract:test).
    const usable = (totalMb - 400) * 0.8
    const extract = Math.max(20, Math.min(300, Math.floor(usable / 9)))
    const test = Math.max(10, Math.floor(extract / 2))
    return { extract, test }
  }

  async function refresh() {
    try {
      setStatus(await warc.status())
    } catch (e) {
      setError((e as Error).message)
    }
  }

  async function refreshR2Health() {
    try {
      const c = await r2.getConfig()
      setR2State(c.state ?? (c.configured ? 'unknown' : 'misconfigured'))
      setR2LastError(c.last_error ?? null)
    } catch {
      // best-effort — leave the pill at its prior state
    }
  }

  async function refreshR2Objects() {
    setR2Listing(true)
    try {
      const r = await r2.listObjects('warc/', 200)
      if (r.ok) setR2Objects(r.objects ?? [])
    } catch {
      // ignore — listing is non-critical
    } finally {
      setR2Listing(false)
    }
  }

  async function onDeleteR2(key: string, accountId?: string, accountLabel?: string) {
    // Account-scoped delete prompt so the operator knows which bucket
    // the row lives in — important now that multiple accounts can hold
    // overlapping prefixes.
    const where = accountLabel ? ` in ${accountLabel}` : ''
    if (!window.confirm(`Delete ${key}${where} from R2? This cannot be undone.`)) return
    notify?.('Deleting R2 object', `${key}${where}`, 'info')
    const res = await r2.deleteObject(key, accountId)
    if (res.ok) {
      notify?.('R2 delete complete', `${key}${where}`, 'info')
      setR2Objects((prev) => prev.filter((o) => !(o.key === key && (!accountId || o.account_id === accountId))))
      // If the dedup pointer was this object, status will re-sync on next poll.
      await refresh()
    } else {
      notify?.('R2 delete failed', res.error ?? 'unknown error', 'error')
    }
  }

  async function onDownloadR2(key: string, accountId?: string) {
    try {
      const res = await r2.downloadUrl(key, accountId)
      if (res.ok && res.url) {
        const a = document.createElement('a')
        a.href = res.url
        a.download = key.split('/').pop() ?? key
        document.body.appendChild(a)
        a.click()
        document.body.removeChild(a)
      } else {
        notify?.('Download failed', res.error ?? 'could not get URL', 'error')
      }
    } catch (e) {
      notify?.('Download failed', (e as Error).message, 'error')
    }
  }

  async function onMergeAll() {
    setR2Merging(true)
    setR2MergeError(null)
    setR2MergeResult(null)
    try {
      const res = await r2.mergeWarc()
      if (res.ok && res.merged_key) {
        setR2MergeResult({ key: res.merged_key, total: res.total_lines ?? 0 })
        notify?.('Merge complete', `${(res.total_lines ?? 0).toLocaleString()} unique domains → ${res.merged_key}`, 'info')
        await refreshR2Objects()
      } else {
        setR2MergeError(res.error ?? 'merge failed')
      }
    } catch (e) {
      setR2MergeError((e as Error).message)
    } finally {
      setR2Merging(false)
    }
  }

  useEffect(() => {
    const refreshHosts = async () => {
      try {
        const r = await warc.hosts()
        // Backend now returns `['controller', ...warc-tagged workers]` already —
        // the earlier version of this component prepended a second 'controller',
        // which is why the dropdown showed 'controller (this VPS)' twice.
        const fromServer = Array.isArray(r?.hosts) && r.hosts.length > 0
          ? r.hosts
          : ['controller']
        setHosts((prev) =>
          prev.length === fromServer.length && prev.every((h, i) => h === fromServer[i])
            ? prev
            : fromServer,
        )
      } catch {
        // Keep whatever we already had — a transient failure shouldn't
        // strand the dropdown back to controller-only.
      }
    }
    void refresh()
    void refreshHosts()
    void refreshR2Health()
    void refreshR2Objects()
    void fetch('/api/system/info').then(r => r.ok ? r.json() : null).then(d => {
      if (d?.total_mb) setSysMem({ total_mb: d.total_mb, available_mb: d.available_mb ?? 0 })
    }).catch(() => {})
    pollTimer.current = window.setInterval(() => {
      void refresh()
      void refreshHosts()
      // R2 health changes slowly — poll at 1/4 the WARC cadence so we stay
      // responsive to operator config changes without burning round-trips.
      if ((Date.now() / POLL_MS) % 4 < 1) void refreshR2Health()
    }, POLL_MS)
    return () => {
      if (pollTimer.current) window.clearInterval(pollTimer.current)
    }
  }, [])

  function togglePreset(id: string) {
    const preset = DOMAIN_PRESETS.find((p) => p.id === id)
    if (!preset) return
    const isActive = activePresets.has(id)
    const next = new Set(activePresets)
    if (isActive) {
      next.delete(id)
      const remove = new Set(preset.domains)
      const kept = crtDomain.split(',').map((d) => d.trim()).filter((d) => d && !remove.has(d))
      setCrtDomain(kept.join(','))
    } else {
      next.add(id)
      const existing = new Set(crtDomain.split(',').map((d) => d.trim()).filter(Boolean))
      preset.domains.forEach((d) => existing.add(d))
      setCrtDomain([...existing].join(','))
    }
    setActivePresets(next)
  }

  async function onStart() {
    setBusy(true); setError(null)
    // Build the source list from the checkboxes. If the operator
    // unchecked both, default to ['cc'] so we never POST a no-producer
    // request — the backend would reject it and the UX would be
    // confusing.
    const sources: string[] = []
    if (sourceCC) sources.push('cc')
    if (sourceCrtSh) sources.push('crtsh')
    if (sources.length === 0) sources.push('cc')

    // Local guard: require at least one pivot if crt.sh is checked, so
    // the operator sees a clear inline message instead of a 400 from
    // the backend.
    if (sourceCrtSh && !crtTld.trim() && !crtDomain.trim()) {
      setError('crt.sh source selected — provide at least one TLD or domain to pivot on.')
      setBusy(false)
      return
    }

    try {
      await warc.start({
        run_on: runOn,
        max_domains: maxDomains,
        extract_workers: extractWorkers,
        test_workers: testWorkers,
        snapshots,
        source: sources,
        crt_tld: sourceCrtSh ? crtTld.trim() : undefined,
        crt_domain: sourceCrtSh ? crtDomain.trim() : undefined,
        subdomain_only: subdomainOnly,
        nodes: ['controller', ...selectedNodes.filter(n => n !== 'controller')],
      })
      await refresh()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function onStop() {
    setBusy(true); setError(null)
    notify?.('Stopping WARC harvest', 'SIGTERM dispatched — status will update on next monitor cycle.', 'info')
    try {
      await warc.stop()
      await refresh()
    } catch (e) {
      const msg = (e as Error).message
      setError(msg)
      notify?.('Stop request failed', msg, 'error')
    } finally {
      setBusy(false)
    }
  }

  async function onExport() {
    setBusy(true); setError(null)
    const count = status?.domains_found ?? 0
    notify?.('Uploading to R2', `Shipping ${count.toLocaleString()} domains to Cloudflare R2…`, 'info')
    try {
      const r = await warc.exportToR2()
      // Name the destination bucket in the toast so the operator knows
      // which account absorbed it (priority spillover means it might
      // not be the "primary" they expect).
      const dest = r.account_label ? ` in ${r.account_label}` : ''
      if (r.noop) {
        notify?.('No upload needed', `${r.message ?? 'content unchanged'} — kept ${r.r2_key}${dest}`, 'info')
      } else {
        notify?.('R2 upload complete', `Saved as ${r.r2_key}${dest}`, 'info')
      }
      await refresh()
      void refreshR2Objects()
    } catch (e) {
      const msg = (e as Error).message
      setError(msg)
      notify?.('R2 upload failed', msg, 'error')
    } finally {
      setBusy(false)
    }
  }

  const running = status?.running === true
  const finishedAt = status?.finished_at
  const r2Key = status?.r2_key
  const r2Error = status?.r2_error
  const exitCode = status?.last_exit_code

  return (
    <section className="warc-panel">
      <header className="card-block__head card-block__head--row">
        <div>
          <h2>WARC harvest</h2>
          <p className="card-block__lede card-block__lede--short">
            <code className="inline-code">warc.go</code> runs on the controller, scans Common Crawl
            archives, tests liveness, writes results to a temp file, then ships them to R2 and
            deletes the local copy — controller disk stays clean.
          </p>
        </div>
        <div className="warc-head-actions">
          <div className="warc-mode">
            <span className={`pill ${running ? 'pill--ok' : 'pill--muted'}`}>
              {running ? 'Running' : 'Idle'}
            </span>
            {running && (
              <span className="pill pill--muted" style={{ fontSize: '0.72rem' }}>
                Running on: {
                  (() => {
                    const ro = status?.run_on ?? 'controller'
                    return ro === 'controller' ? 'controller' : labelForHost(ro, fleet)
                  })()
                }
              </span>
            )}
            {r2Key && (
              <span className="pill pill--ok" title={r2Key} style={{ fontSize: '0.72rem' }}>
                ↑ R2
              </span>
            )}
            {(() => {
              const cls =
                r2State === 'connected' ? 'pill--ok'
                : r2State === 'misconfigured' ? 'pill--warn'
                : r2State === 'unreachable' ? 'pill--danger'
                : 'pill--muted'
              const label =
                r2State === 'connected' ? '● R2 connected'
                : r2State === 'misconfigured' ? '● R2 not configured'
                : r2State === 'unreachable' ? '● R2 unreachable'
                : '● R2 …'
              return (
                <span
                  className={`pill ${cls}`}
                  style={{ fontSize: '0.72rem' }}
                  title={r2LastError ?? (r2State === 'connected' ? 'head_bucket ok on last probe' : 'See R2 Settings')}
                >
                  {label}
                </span>
              )
            })()}
          </div>
          <div className="warc-controls">
            {running ? (
              <button type="button" className="btn-danger-outline" onClick={() => void onStop()} disabled={busy}>
                ■ Stop harvest
              </button>
            ) : (
              <button type="button" className="btn-glass" onClick={() => void onStart()} disabled={busy}>
                ▶ Start harvest
              </button>
            )}
            <button
              type="button"
              className="btn-glass"
              onClick={() => void onExport()}
              disabled={busy || !status?.domains_found}
              title={status?.domains_found ? `Push ${status.domains_found} domains to R2 now` : 'Nothing harvested yet'}
            >
              Export to R2 ({status?.domains_found ?? 0})
            </button>
          </div>
        </div>
      </header>

      {!running && (
        <div className="kv kv--form" style={{ marginTop: '1rem' }}>
          <div className="kv__row">
            <label className="kv__label" htmlFor="warc-runon">Run on</label>
            <select
              id="warc-runon"
              className="kv__input"
              value={runOn}
              onChange={(e) => setRunOn(e.target.value)}
            >
              {hosts.map((h) => (
                <option key={h} value={h}>
                  {h === 'controller' ? 'controller (this VPS)' : labelForHost(h, fleet)}
                </option>
              ))}
            </select>
          </div>
          <div className="kv__row">
            <label className="kv__label" htmlFor="warc-max">Max live domains</label>
            <input
              id="warc-max"
              type="number"
              min={100}
              className="kv__input"
              value={maxDomains}
              onChange={(e) => setMaxDomains(Math.max(100, Number(e.target.value) || 10000))}
            />
          </div>
          <div className="kv__row">
            <label className="kv__label" htmlFor="warc-ext">Extract workers</label>
            <input
              id="warc-ext"
              type="number"
              min={1}
              max={500}
              className="kv__input"
              value={extractWorkers}
              onChange={(e) => setExtractWorkers(Math.max(1, Number(e.target.value) || 50))}
            />
          </div>
          <div className="kv__row">
            <label className="kv__label" htmlFor="warc-test">Liveness workers</label>
            <input
              id="warc-test"
              type="number"
              min={1}
              max={500}
              className="kv__input"
              value={testWorkers}
              onChange={(e) => setTestWorkers(Math.max(1, Number(e.target.value) || 25))}
            />
          </div>
          {sysMem && (() => {
            const gb = (sysMem.total_mb / 1024).toFixed(1)
            const s = suggestWorkers(sysMem.total_mb)
            const alreadyTuned = extractWorkers === s.extract && testWorkers === s.test
            return (
              <div className="kv__row">
                <span className="kv__label" style={{ fontSize: '0.78rem', color: 'var(--text-muted)' }}>
                  VPS: {gb} GB RAM
                </span>
                {alreadyTuned ? (
                  <span style={{ fontSize: '0.78rem', color: 'var(--accent)', opacity: 0.85 }}>
                    ✓ tuned for {gb} GB
                  </span>
                ) : (
                  <button
                    type="button"
                    onClick={() => { setExtractWorkers(s.extract); setTestWorkers(s.test) }}
                    style={{
                      fontSize: '0.78rem',
                      padding: '2px 10px',
                      borderRadius: '999px',
                      border: '1px solid var(--accent)',
                      background: 'transparent',
                      color: 'var(--accent)',
                      cursor: 'pointer',
                      lineHeight: 1.6,
                    }}
                    title={`Set extract=${s.extract}, test=${s.test} — safe for ${gb} GB`}
                  >
                    Tune for {gb} GB → {s.extract}/{s.test}
                  </button>
                )}
              </div>
            )
          })()}
          <div className="kv__row">
            <label className="kv__label" htmlFor="warc-snapshots">CC-MAIN snapshots</label>
            <input
              id="warc-snapshots"
              type="number"
              min={0}
              max={20}
              className="kv__input"
              value={snapshots}
              onChange={(e) => setSnapshots(Math.max(0, Math.min(20, Number(e.target.value) || 0)))}
              title="0 = auto (1 snapshot for <500k domains, 2 for <1M, 3 for >1M). Set 1–20 to override."
            />
          </div>
          {/* ── Producer source selection ────────────────────────────
              Two independent checkboxes (not a radio): the operator
              may enable CC, crt.sh, or both in the same harvest. */}
          <div className="kv__row">
            <span className="kv__label">Sources</span>
            <div style={{ display: 'flex', gap: '1rem', alignItems: 'center', flexWrap: 'wrap' }}>
              <label style={{ display: 'inline-flex', alignItems: 'center', gap: '.35rem', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={sourceCC}
                  onChange={(e) => setSourceCC(e.target.checked)}
                />
                <span>Common Crawl</span>
              </label>
              <label style={{ display: 'inline-flex', alignItems: 'center', gap: '.35rem', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={sourceCrtSh}
                  onChange={(e) => setSourceCrtSh(e.target.checked)}
                />
                <span>crt.sh</span>
              </label>
            </div>
          </div>
          {sourceCrtSh && (
            <>
              <div className="kv__row">
                <label className="kv__label" htmlFor="warc-crt-tld">crt.sh TLDs</label>
                <input
                  id="warc-crt-tld"
                  type="text"
                  className="kv__input"
                  placeholder="com,net,io"
                  value={crtTld}
                  onChange={(e) => setCrtTld(e.target.value)}
                  title="Comma-separated TLD list, e.g. com,net,io"
                />
              </div>
              <div className="kv__row">
                <label className="kv__label" htmlFor="warc-crt-domain">crt.sh domains</label>
                <input
                  id="warc-crt-domain"
                  type="text"
                  className="kv__input"
                  placeholder="example.com,foo.io — or use category presets below"
                  value={crtDomain}
                  onChange={(e) => setCrtDomain(e.target.value)}
                  title="Comma-separated registered-domain list (e.g. example.com,foo.io)"
                />
              </div>
              <div className="kv__row" style={{ alignItems: 'flex-start' }}>
                <span className="kv__label" style={{ paddingTop: '.3rem', flexShrink: 0 }}>Presets</span>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '.3rem' }}>
                  {DOMAIN_PRESETS.map((p) => {
                    const on = activePresets.has(p.id)
                    return (
                      <button
                        key={p.id}
                        type="button"
                        onClick={() => togglePreset(p.id)}
                        title={`${p.domains.length} domains${on ? ' — click to remove' : ' — click to add'}`}
                        style={{
                          padding: '.18rem .52rem',
                          fontSize: '.7rem',
                          borderRadius: '2rem',
                          border: `1px solid ${on ? 'var(--accent, #6cc6ff)' : 'rgba(255,255,255,.15)'}`,
                          background: on ? 'rgba(108,198,255,.15)' : 'transparent',
                          color: on ? 'var(--accent, #6cc6ff)' : 'rgba(255,255,255,.55)',
                          cursor: 'pointer',
                          transition: 'all .12s',
                          whiteSpace: 'nowrap',
                        }}
                      >
                        {on ? '✓ ' : ''}{p.label}
                        <span style={{ opacity: .55, marginLeft: '.25rem' }}>{p.domains.length}</span>
                      </button>
                    )
                  })}
                </div>
              </div>
            </>
          )}
          <div className="kv__row">
            <span className="kv__label">Filter</span>
            <label style={{ display: 'inline-flex', alignItems: 'center', gap: '.35rem', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={subdomainOnly}
                onChange={(e) => setSubdomainOnly(e.target.checked)}
              />
              <span>Subdomain only (drop apex / eTLD+1 entries)</span>
            </label>
          </div>
          {/* Distributed nodes selector */}
          {fleet && fleet.length > 0 && (
            <div className="kv__row">
              <span className="kv__label">Nodes</span>
              <div style={{ display: 'flex', gap: '.75rem', flexWrap: 'wrap', alignItems: 'center' }}>
                {/* Controller is always included */}
                <label style={{ display: 'inline-flex', alignItems: 'center', gap: '.35rem', cursor: 'default', fontSize: '.85rem', opacity: 0.7 }}>
                  <input type="checkbox" checked disabled />
                  <span>controller</span>
                </label>
                {fleet.map(node => {
                  const nodeId = node.host ?? node.id
                  const label = node.label ? `${node.label} (${nodeId})` : nodeId
                  const checked = selectedNodes.includes(nodeId)
                  return (
                    <label key={nodeId} style={{ display: 'inline-flex', alignItems: 'center', gap: '.35rem', cursor: 'pointer', fontSize: '.85rem' }}>
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={e => setSelectedNodes(prev =>
                          e.target.checked ? [...prev, nodeId] : prev.filter(n => n !== nodeId)
                        )}
                      />
                      <span>{label}</span>
                    </label>
                  )
                })}
              </div>
            </div>
          )}
        </div>
      )}

      {status?.running === true
        && status?.domains_found === 0
        && status?.started_at
        && (Date.now() - new Date(status.started_at).getTime() < 60000) && (
        <div
          className="muted"
          style={{
            marginTop: '1rem',
            padding: '.55rem .75rem',
            background: 'rgba(255,255,255,.03)',
            borderRadius: '.4rem',
            fontSize: '.78rem',
            fontStyle: 'italic',
          }}
        >
          Initialising — fetching CC-MAIN snapshots (~30s)…
        </div>
      )}

      <div style={{ marginTop: '1rem', display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '.75rem' }}>
        <Stat label="Domains found" value={status?.domains_found ?? 0} />
        {/* TARGET tile — click-to-edit when idle. While a harvest is in
            flight, max-domains is locked into the running process; editing
            here only matters for the *next* Start. We surface this via the
            disabled state + title attr instead of silently no-op'ing. */}
        <EditableStat
          label="Target"
          value={maxDomains}
          locked={running}
          onCommit={(v) => setMaxDomains(Math.max(1, v))}
          lockedHint="Locked while harvest is running — Stop to change for next run"
          editHint="Click to set the next harvest's target"
        />
        <Stat
          label="Started"
          value={status?.started_at ? new Date(status.started_at).toLocaleTimeString() : '—'}
        />
        <Stat
          label="Status"
          value={
            running
              ? 'Harvesting'
              : finishedAt
                ? exitCode === 0
                  ? 'Completed'
                  : `Exited (${exitCode})`
                : 'Idle'
          }
        />
      </div>

      {/* Detailed harvest stats — parsed from the latest [PROGRESS] line
          in log_tail. Hidden when no progress lines exist yet (clean Idle
          state before the first run) so the cockpit doesn't show a row of
          zeros to a confused operator. */}
      {(() => {
        const prog = parseLatestProgress(status?.log_tail ?? [])
        if (!prog && !running && !finishedAt) return null
        const live = prog?.live ?? status?.domains_found ?? 0
        const target = prog?.target ?? status?.max_domains ?? maxDomains
        const tested = prog?.tested ?? 0
        const extracted = prog?.extracted ?? 0
        const filesDone = prog?.filesDone ?? 0
        const filesTotal = prog?.filesTotal ?? 0
        const startedMs = status?.started_at ? new Date(status.started_at).getTime() : 0
        const finishedMs = finishedAt ? new Date(finishedAt).getTime() : Date.now()
        const elapsedSec = startedMs > 0 ? Math.max(1, (finishedMs - startedMs) / 1000) : 0
        const rate = elapsedSec > 0 ? live / elapsedSec : 0
        const hitRate = tested > 0 ? (live / tested) * 100 : 0
        const remaining = Math.max(0, target - live)
        const etaSec = rate > 0 ? remaining / rate : 0
        return (
          <div style={{ marginTop: '.6rem', display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '.75rem' }}>
            <Stat label="Tested" value={tested.toLocaleString()} />
            <Stat label="Extracted" value={extracted.toLocaleString()} />
            <Stat
              label="WARC files"
              value={filesTotal > 0 ? `${filesDone}/${filesTotal}` : '—'}
            />
            <Stat
              label="Hit rate"
              value={tested > 0 ? `${hitRate.toFixed(1)}%` : '—'}
            />
            <Stat
              label="Rate"
              value={rate > 0 ? `${rate.toFixed(1)} live/s` : '—'}
            />
            <Stat
              label="Elapsed"
              value={elapsedSec > 0 ? formatDuration(elapsedSec) : '—'}
            />
            <Stat
              label="ETA"
              value={
                running && rate > 0 && remaining > 0
                  ? formatDuration(etaSec)
                  : !running && finishedAt
                    ? 'done'
                    : '—'
              }
            />
            <Stat
              label="Progress"
              value={target > 0 ? `${Math.min(100, (live / target) * 100).toFixed(1)}%` : '—'}
            />
          </div>
        )
      })()}

      {(error || r2Error) && (
        <p className={`settings-hint ${r2Error ? 'tg-hint--err' : ''}`} style={{ marginTop: '.75rem' }}>
          {error || r2Error}
        </p>
      )}

      {r2Key && (
        <div className="kv" style={{ marginTop: '.75rem' }}>
          <div className="kv__row">
            <span className="kv__label muted">Saved to R2</span>
            <span className="mono" style={{ fontSize: '.82rem', wordBreak: 'break-all' }}>{r2Key}</span>
          </div>
          {status?.r2_live_key && (
            <div className="kv__row" style={{ marginTop: '.5rem', fontSize: '.82rem' }}>
              <span className="kv__label muted">Live snapshot</span>
              <span className="mono" style={{ fontSize: '.78rem', color: 'var(--accent)', opacity: 0.85 }}>
                {status.r2_live_key}
                {status.r2_live_uploaded_at && (
                  <span className="muted" style={{ marginLeft: '.5rem' }}>
                    @ {new Date(status.r2_live_uploaded_at).toLocaleTimeString()}
                  </span>
                )}
              </span>
            </div>
          )}
        </div>
      )}
      {!r2Key && status?.r2_live_key && (
        <div className="kv" style={{ marginTop: '.75rem' }}>
          <div className="kv__row" style={{ fontSize: '.82rem' }}>
            <span className="kv__label muted">Live snapshot</span>
            <span className="mono" style={{ fontSize: '.78rem', color: 'var(--accent)', opacity: 0.85 }}>
              {status.r2_live_key}
              {status.r2_live_uploaded_at && (
                <span className="muted" style={{ marginLeft: '.5rem' }}>
                  @ {new Date(status.r2_live_uploaded_at).toLocaleTimeString()}
                </span>
              )}
            </span>
          </div>
        </div>
      )}

      {status?.dist_nodes && status.dist_nodes.length > 0 && (
        <div style={{ marginTop: '1rem' }}>
          <p className="muted" style={{ fontSize: '.78rem', marginBottom: '.4rem' }}>Distributed nodes</p>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))', gap: '.5rem' }}>
            {status.dist_nodes.map(nd => (
              <div key={nd.id} style={{
                background: 'rgba(255,255,255,.04)',
                border: '1px solid var(--hairline)',
                borderRadius: '.4rem',
                padding: '.4rem .6rem',
                fontSize: '.78rem',
              }}>
                <div className="mono" style={{ color: 'var(--text)' }}>{nd.id}</div>
                <div className="muted">{nd.status ?? '—'}</div>
                {nd.max_domains && <div className="muted">target: {nd.max_domains.toLocaleString()}</div>}
                {nd.error && <div style={{ color: 'var(--danger)', fontSize: '.72rem' }}>{nd.error}</div>}
              </div>
            ))}
          </div>
        </div>
      )}

      <div style={{ marginTop: '1rem' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '.6rem' }}>
          <div className="muted" style={{ fontSize: '.78rem' }}>
            R2 exports {r2Objects.length > 0 ? `(${r2Objects.length})` : ''}
          </div>
          <div style={{ display: 'flex', gap: '.4rem' }}>
            <button
              type="button"
              className="btn-glass btn-glass--xs"
              onClick={() => void onMergeAll()}
              disabled={r2Merging || r2Objects.length === 0}
              title="Merge + deduplicate all warc/ lists into one file in R2"
            >
              {r2Merging ? '⏳ Merging…' : '⊕ Merge All'}
            </button>
            <button
              type="button"
              className="btn-glass btn-glass--xs"
              onClick={() => void refreshR2Objects()}
              disabled={r2Listing}
              title="Re-list objects in the warc/ prefix"
            >
              {r2Listing ? '…' : '↻'}
            </button>
          </div>
        </div>
        {r2MergeError && (
          <p style={{ color: 'var(--color-danger, #f66)', fontSize: '.72rem', marginTop: '.3rem' }}>
            Merge failed: {r2MergeError}
          </p>
        )}
        {r2MergeResult && (
          <div style={{ display: 'flex', alignItems: 'center', gap: '.6rem', marginTop: '.35rem' }}>
            <span className="muted" style={{ fontSize: '.72rem' }}>
              ✓ {r2MergeResult.total.toLocaleString()} unique domains merged
            </span>
            <button
              type="button"
              className="btn-glass btn-glass--xs"
              onClick={() => void onDownloadR2(r2MergeResult.key)}
              title={`Download ${r2MergeResult.key}`}
            >
              ↓ Download merged
            </button>
          </div>
        )}
        {r2Objects.length === 0 ? (
          <p className="muted" style={{ fontSize: '.72rem', marginTop: '.3rem' }}>
            {r2State === 'connected' ? 'No exports yet.' : 'Connect R2 in Settings to see exports.'}
          </p>
        ) : (
          <table className="fleet-boot__table" style={{ marginTop: '.4rem', fontSize: '.74rem' }}>
            <thead>
              <tr>
                <th>Key</th>
                <th>Account</th>
                <th style={{ textAlign: 'right' }}>Size</th>
                <th>Modified</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {r2Objects.map((o) => (
                <tr key={`${o.account_id ?? ''}::${o.key}`}>
                  <td className="mono" style={{ wordBreak: 'break-all' }}>{o.key}</td>
                  <td>
                    {o.account_label ? (
                      <span
                        className="pill pill--muted"
                        style={{ fontSize: '.7rem' }}
                        title={`Stored in R2 account "${o.account_label}"`}
                      >
                        {o.account_label}
                      </span>
                    ) : (
                      <span className="muted">—</span>
                    )}
                  </td>
                  <td className="mono" style={{ textAlign: 'right' }}>{(o.size / 1024).toFixed(1)} KB</td>
                  <td className="muted">{o.modified ? new Date(o.modified).toLocaleString() : '—'}</td>
                  <td>
                    <div style={{ display: 'flex', gap: '.35rem' }}>
                      <button
                        type="button"
                        className="btn-glass btn-glass--xs"
                        style={{ fontSize: '.7rem' }}
                        onClick={() => void onDownloadR2(o.key, o.account_id)}
                        title={`Download ${o.key}`}
                      >
                        ↓
                      </button>
                      <button
                        type="button"
                        className="btn-danger-outline"
                        style={{ fontSize: '.7rem', padding: '.15rem .5rem' }}
                        onClick={() => void onDeleteR2(o.key, o.account_id, o.account_label)}
                        title={`Delete ${o.key} from R2${o.account_label ? ` (${o.account_label})` : ''}`}
                      >
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {status?.log_tail && status.log_tail.length > 0 && (
        <details style={{ marginTop: '1rem' }}>
          <summary className="muted" style={{ cursor: 'pointer', fontSize: '.8rem' }}>
            Last {status.log_tail.length} log lines
          </summary>
          <pre
            className="mono"
            style={{
              fontSize: '.72rem',
              maxHeight: '14rem',
              overflowY: 'auto',
              background: 'rgba(0,0,0,.35)',
              padding: '.6rem .8rem',
              borderRadius: '.4rem',
              marginTop: '.4rem',
            }}
          >
            {status.log_tail.join('\n')}
          </pre>
        </details>
      )}
    </section>
  )
}

/** Parse the most recent `[PROGRESS] Live: 18/10000 | Tested: 76 | Extracted:
 * 3636 | Files: 0/200` line emitted by warc.go. ANSI colour escapes are
 * stripped so the regex stays simple. Returns null when no progress line is
 * present (clean Idle state pre-first-run). */
function parseLatestProgress(lines: readonly string[]): {
  live: number; target: number; tested: number; extracted: number
  filesDone: number; filesTotal: number
} | null {
  if (!lines || lines.length === 0) return null
  const stripAnsi = (s: string) => s.replace(/\[[0-9;]*m/g, '')
  const re = /\[PROGRESS\]\s+Live:\s+(\d+)\/(\d+)\s+\|\s+Tested:\s+(\d+)\s+\|\s+Extracted:\s+(\d+)\s+\|\s+Files:\s+(\d+)\/(\d+)/
  // Walk lines in reverse — the most recent progress line wins. A single
  // log entry can hold many concatenated progress chunks (\r-based progress
  // bar in warc.go), so we also re-scan inside each entry with a global
  // regex and take the last match.
  for (let i = lines.length - 1; i >= 0; i--) {
    const cleaned = stripAnsi(lines[i] ?? '')
    let last: RegExpExecArray | null = null
    const g = new RegExp(re, 'g')
    let m: RegExpExecArray | null
    while ((m = g.exec(cleaned)) !== null) last = m
    if (last) {
      return {
        live: Number(last[1]) || 0,
        target: Number(last[2]) || 0,
        tested: Number(last[3]) || 0,
        extracted: Number(last[4]) || 0,
        filesDone: Number(last[5]) || 0,
        filesTotal: Number(last[6]) || 0,
      }
    }
  }
  return null
}

/** "1h 23m 04s" / "23m 04s" / "04s" — used by Elapsed and ETA tiles. */
function formatDuration(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (h > 0) return `${h}h ${String(m).padStart(2, '0')}m ${String(sec).padStart(2, '0')}s`
  if (m > 0) return `${m}m ${String(sec).padStart(2, '0')}s`
  return `${sec}s`
}

function Stat({ label, value }: { label: string; value: number | string }) {
  return (
    <div style={{ background: 'rgba(255,255,255,.03)', borderRadius: '.4rem', padding: '.55rem .7rem' }}>
      <div className="muted" style={{ fontSize: '.65rem', textTransform: 'uppercase', letterSpacing: '.06em' }}>{label}</div>
      <div className="mono" style={{ fontSize: '1rem', marginTop: '.15rem' }}>{value}</div>
    </div>
  )
}

/** Same shape as Stat but the value cell turns into an input on click.
 * Used for TARGET so the operator can change the next run's max-domains
 * without scrolling back to the form. Locked while a harvest is running. */
function EditableStat({
  label, value, locked, lockedHint, editHint, onCommit,
}: {
  label: string
  value: number
  locked: boolean
  lockedHint: string
  editHint: string
  onCommit: (next: number) => void
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState<string>(String(value))
  // Keep the draft in sync if the parent value updates (e.g., after a Start
  // resets it). Without this the field would silently drift.
  useEffect(() => { if (!editing) setDraft(String(value)) }, [value, editing])

  const commit = () => {
    const n = Number(draft)
    if (Number.isFinite(n) && n > 0) onCommit(Math.floor(n))
    setEditing(false)
  }

  return (
    <div
      style={{
        background: 'rgba(255,255,255,.03)',
        borderRadius: '.4rem',
        padding: '.55rem .7rem',
        cursor: locked ? 'not-allowed' : 'pointer',
        outline: editing ? '1px solid var(--accent, #6cc6ff)' : 'none',
      }}
      title={locked ? lockedHint : editHint}
      onClick={() => { if (!locked && !editing) setEditing(true) }}
    >
      <div className="muted" style={{ fontSize: '.65rem', textTransform: 'uppercase', letterSpacing: '.06em' }}>
        {label} {locked ? '🔒' : editing ? '✎' : ''}
      </div>
      {editing ? (
        <input
          autoFocus
          type="number"
          min={1}
          className="mono"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === 'Enter') commit()
            else if (e.key === 'Escape') { setDraft(String(value)); setEditing(false) }
          }}
          style={{
            fontSize: '1rem', marginTop: '.15rem', width: '100%',
            background: 'transparent', color: 'inherit',
            border: 'none', outline: 'none', padding: 0,
          }}
        />
      ) : (
        <div className="mono" style={{ fontSize: '1rem', marginTop: '.15rem' }}>
          {value.toLocaleString()}
        </div>
      )}
    </div>
  )
}
