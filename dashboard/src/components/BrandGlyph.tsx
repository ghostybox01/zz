import type { ComponentType, ReactElement, SVGProps } from 'react'
import { siStripe } from 'simple-icons'

type Common = SVGProps<SVGSVGElement>

const box: Common = { width: 28, height: 28, viewBox: '0 0 32 32', fill: 'none' }
const box24: Common = { width: 28, height: 28, viewBox: '0 0 24 24', fill: 'none' }

/** AWS Simple Email Service — orange "AWS" wedge above an envelope. */
export function GlyphAwsSes(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="11" width="26" height="16" rx="2.5" fill="#232f3e" stroke="#ff9900" strokeWidth="1.6"/>
      <path d="M4 12.5l12 8.5 12-8.5" stroke="#ff9900" strokeWidth="1.6" strokeLinejoin="round" fill="none"/>
      <path d="M9 7l7-4 7 4" stroke="#ff9900" strokeWidth="1.6" strokeLinejoin="round" fill="none"/>
      <circle cx="16" cy="6" r="1.3" fill="#ff9900"/>
    </svg>
  )
}

/** SendGrid — blue layered diamond. */
export function GlyphSendGrid(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3"  y="3"  width="11" height="11" rx="1.2" fill="#9dd4f3"/>
      <rect x="14" y="3"  width="15" height="11" rx="1.2" fill="#1a82e2"/>
      <rect x="3"  y="14" width="15" height="15" rx="1.2" fill="#1a82e2"/>
      <rect x="18" y="14" width="11" height="11" rx="1.2" fill="#9dd4f3"/>
    </svg>
  )
}

/** Stripe — official mark from simple-icons (MIT). */
export function GlyphStripe(props: Common) {
  return (
    <svg {...box24} {...props}>
      <rect width="24" height="24" rx="4" fill={`#${siStripe.hex}`}/>
      <g transform="translate(4 4) scale(0.667)">
        <path d={siStripe.path} fill="#ffffff"/>
      </g>
    </svg>
  )
}

/** Twilio — red rounded square with white "T" mark. */
export function GlyphTwilio(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="13" fill="#f22f46"/>
      <circle cx="11.5" cy="11.5" r="2.6" fill="#fff"/>
      <circle cx="20.5" cy="11.5" r="2.6" fill="#fff"/>
      <circle cx="11.5" cy="20.5" r="2.6" fill="#fff"/>
      <circle cx="20.5" cy="20.5" r="2.6" fill="#fff"/>
    </svg>
  )
}

/** AWS Deep — orange chevron + smile cloud. */
export function GlyphAwsDeep(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="5" fill="#232f3e"/>
      <path d="M7 21c2 2 6 3 9 3s7-1 9-3" stroke="#ff9900" strokeWidth="2" fill="none" strokeLinecap="round"/>
      <path d="M21 22l3 1-1 3" stroke="#ff9900" strokeWidth="2" fill="none" strokeLinecap="round" strokeLinejoin="round"/>
      <path d="M11 11h10M11 14h7M11 17h10" stroke="#ff9900" strokeWidth="1.6" strokeLinecap="round"/>
    </svg>
  )
}

/** OpenAI — black hexagon, white knot. */
export function GlyphOpenAI(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="5" fill="#10a37f"/>
      <path d="M16 9.5l5 2.9v5.9l-5 2.9-5-2.9v-5.9l5-2.9z" stroke="#fff" strokeWidth="1.6" fill="none"/>
      <path d="M11 12.4l5 2.9 5-2.9M16 21.2v-5.9" stroke="#fff" strokeWidth="1.6"/>
    </svg>
  )
}

/** Anthropic — wheat hexagon with letter A wedge. */
export function GlyphAnthropic(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="5" fill="#cd9d6c"/>
      <path d="M10 22 16 9l6 13h-3.4l-1.3-3h-2.6l-1.3 3H10z" fill="#1a120a"/>
      <path d="M14.9 17h2.2L16 14.3 14.9 17z" fill="#cd9d6c"/>
    </svg>
  )
}

/** GitHub — dark octocat silhouette. */
export function GlyphGitHub(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="5" fill="#181717"/>
      <path d="M16 8a8 8 0 0 0-2.5 15.6c.4.07.55-.17.55-.38v-1.4c-2.25.5-2.7-1-2.7-1-.4-.92-.95-1.16-.95-1.16-.78-.53.06-.52.06-.52.86.06 1.3.88 1.3.88.76 1.3 2 .93 2.5.7.07-.55.3-.92.55-1.13-1.8-.2-3.7-.9-3.7-4 0-.88.31-1.6.83-2.17-.08-.2-.36-1.03.08-2.15 0 0 .67-.21 2.2.83a7.6 7.6 0 0 1 4 0c1.53-1.04 2.2-.83 2.2-.83.44 1.12.16 1.95.08 2.15.51.57.83 1.3.83 2.17 0 3.1-1.9 3.79-3.71 3.99.31.27.58.79.58 1.6v2.36c0 .22.15.46.55.38A8 8 0 0 0 16 8Z" fill="#fff"/>
    </svg>
  )
}

/** Mailgun — red M. */
export function GlyphMailgun(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="5" fill="#c02e1d"/>
      <circle cx="16" cy="16" r="7" stroke="#fff" strokeWidth="1.6" fill="none"/>
      <circle cx="16" cy="16" r="3" fill="#fff"/>
      <path d="M22 21l3 3" stroke="#fff" strokeWidth="1.6" strokeLinecap="round"/>
    </svg>
  )
}

/** Brevo — green origami swallow. */
export function GlyphBrevo(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="5" fill="#0b996e"/>
      <path d="M9 22l7-13 7 13-7-3-7 3z" fill="#fff"/>
      <path d="M16 9v10" stroke="#0b996e" strokeWidth="1.2"/>
    </svg>
  )
}

/** Mandrill — orange dust + M. */
export function GlyphMandrill(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="5" fill="#e87b35"/>
      <path d="M9 22V10l4 7 3-5 3 5 4-7v12" stroke="#fff" strokeWidth="2.2" fill="none" strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  )
}

/** GCP — multicolour polygon. */
export function GlyphGcp(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="5" fill="#0f1f3a"/>
      <path d="M11 19.4l-2.6-4.6L11 10.2h4l1.3 2.3-2.2 3.8H16l2.2-3.8L20.5 16l-2.6 4.6h-4L11 19.4z" fill="#4285f4"/>
      <path d="M16 12.5l-1.3 2.3 1.3 2.3 1.3-2.3-1.3-2.3z" fill="#fbbc04"/>
      <path d="M20.5 16l-1.6 2.8 2 1.2 1.6-2.7-2-1.3z" fill="#34a853"/>
      <path d="M16 7l-3 2 3 2 3-2-3-2z" fill="#ea4335"/>
    </svg>
  )
}

/** SSRF probe — shield + arrow. */
export function GlyphSsrf(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="5" fill="#1f2433"/>
      <path d="M16 6 8 9v6c0 5 4 8 8 10 4-2 8-5 8-10V9l-8-3z" stroke="#ff8a3d" strokeWidth="1.8" fill="none"/>
      <path d="M12 16h7m-2-3 3 3-3 3" stroke="#ff8a3d" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  )
}

/** AI Keys — neural-net glyph covering Claude / GPT / Gemini / xAI / Mistral. */
export function GlyphAI(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="7" fill="#1f2433"/>
      <circle cx="11" cy="11" r="2.2" fill="#a78bfa"/>
      <circle cx="21" cy="11" r="2.2" fill="#34d399"/>
      <circle cx="11" cy="21" r="2.2" fill="#fbbf24"/>
      <circle cx="21" cy="21" r="2.2" fill="#60a5fa"/>
      <circle cx="16" cy="16" r="3" fill="#fff"/>
      <path d="M11 11l5 5M21 11l-5 5M11 21l5-5M21 21l-5-5" stroke="#fff" strokeWidth="1.1" opacity="0.7"/>
    </svg>
  )
}

/** Random SMTP — envelope + tilde to imply "any/random". */
export function GlyphSmtp(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="6" fill="#1f2433"/>
      <rect x="6" y="10" width="20" height="13" rx="2" stroke="#fbbf24" strokeWidth="1.6" fill="none"/>
      <path d="M7 11l9 7 9-7" stroke="#fbbf24" strokeWidth="1.6" strokeLinejoin="round" fill="none"/>
      <path d="M9 24c1-1 2-1 3 0s2 1 3 0 2-1 3 0 2 1 3 0" stroke="#34d399" strokeWidth="1.4" fill="none" strokeLinecap="round"/>
    </svg>
  )
}

/** LFI probe — file with arrow back. */
export function GlyphLfi(props: Common) {
  return (
    <svg {...box} {...props}>
      <rect x="3" y="3" width="26" height="26" rx="5" fill="#1f2433"/>
      <path d="M11 8h7l3 3v13H11z" stroke="#fbbf24" strokeWidth="1.6" fill="none" strokeLinejoin="round"/>
      <path d="M18 8v3h3" stroke="#fbbf24" strokeWidth="1.6"/>
      <text x="13.5" y="21" fontSize="6" fontFamily="monospace" fill="#fbbf24">..</text>
    </svg>
  )
}

/* ─────────────────────────────────────────────────────────────────
 * Brandfetch override hook
 *
 * If you paste a Brandfetch Client ID into VITE_BRANDFETCH_CLIENT_ID
 * (in .env.local), `<BrandLogo domain="sendgrid.com" />` will fetch
 * the official logo via Brandfetch CDN. Otherwise it falls back to
 * the inline SVG passed as `Fallback`.
 *
 * Free Brandfetch Client IDs: https://brandfetch.com/developers
 * ─────────────────────────────────────────────────────────────── */

type BrandLogoProps = {
  domain: string
  Fallback: ComponentType<Common>
  alt: string
  size?: number
}

// Suppress unused-export warning for ReactElement (kept for typing clarity)
export type _BrandLogoReturn = ReactElement

const BRANDFETCH_ID =
  (import.meta as unknown as { env?: { VITE_BRANDFETCH_CLIENT_ID?: string } }).env
    ?.VITE_BRANDFETCH_CLIENT_ID ?? ''

export function BrandLogo({ domain, Fallback, alt, size = 42 }: BrandLogoProps) {
  if (BRANDFETCH_ID) {
    return (
      <img
        src={`https://cdn.brandfetch.io/${domain}?c=${BRANDFETCH_ID}`}
        alt={alt}
        width={size}
        height={size}
        loading="lazy"
        style={{ borderRadius: 8 }}
        onError={(e) => {
          // If Brandfetch returns 4xx/5xx, swap in the fallback inline SVG
          ;(e.currentTarget as HTMLImageElement).style.display = 'none'
        }}
      />
    )
  }
  return <Fallback width={size} height={size} />
}
