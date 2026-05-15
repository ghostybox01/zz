import type { TargetList } from '../types'

const now = Date.now()

export const demoListsSeed: readonly TargetList[] = [
  {
    id: 'list-q3-portfolio',
    name: 'q3-portfolio.txt',
    uploadedAt: new Date(now - 4 * 60 * 60_000).toISOString(),
    lineCount: 50_000,
    contentHash: 'demo-q3p-50000',
    preview: [
      'edge.tenant.io',
      'cdn-prod.tenant.io',
      'staging.shop.tenant.io',
      'beta.acme.shop',
      'api.tenant.io',
      'auth.tenant.io',
    ],
    assignedVpsIds: ['vps-ams-1', 'vps-ams-2'],
    status: 'deployed',
    note: 'Q3 portfolio sweep — split AMS workers',
  },
  {
    id: 'list-edu-sweep',
    name: 'edu-sweep.txt',
    uploadedAt: new Date(now - 90 * 60_000).toISOString(),
    lineCount: 12_400,
    contentHash: 'demo-edu-12400',
    preview: [
      'k12.example.test',
      'students.example.test',
      'lms.example.test',
      'libraries.example.test',
      'campus.example.test',
      'records.example.test',
    ],
    assignedVpsIds: ['vps-sgp-1'],
    status: 'deployed',
    note: 'k12 + .edu portfolio',
  },
  {
    id: 'list-mail-attic',
    name: 'mail-attic-rescan.txt',
    uploadedAt: new Date(now - 3 * 24 * 60 * 60_000).toISOString(),
    lineCount: 8_900,
    contentHash: 'demo-attic-8900',
    preview: [
      'mailer.legacy.test',
      'old.acme.test',
      'archive.acme.test',
      'beta.acme.test',
      'staging.acme.test',
      'feed.acme.test',
    ],
    assignedVpsIds: [],
    status: 'completed',
    note: 'Brevo / SG / Mailgun re-scan — done last week',
  },
]
