import i18n from 'i18next'

/**
 * The API's refusal (RFC 9457 problem details). `field` names the request
 * member at fault — a member inside an object by its path
 * (`settings.allowed_domains`).
 */
export interface ServerRefusal {
  code?: string
  field?: string
  detail?: string
  hint?: string
}

/** The refusal behind a failed request, or null when the API gave none. */
export function serverRefusal(error: unknown): ServerRefusal | null {
  const data = (error as { response?: { data?: unknown } } | null | undefined)?.response?.data
  return data && typeof data === 'object' ? (data as ServerRefusal) : null
}

// Codes that stand for a whole family of refusals, each worded by the API for
// the case at hand. Their translation is only the family's name; when the
// refusal also names the request member at fault, the API's own words say more.
const FAMILY_CODES = new Set([
  'invalid_request',
  'internal_error',
  'internal_server_error',
  'forbidden',
  'insufficient_scope',
])

/** The translation of a refusal's code, when the console has one that fits. */
export function refusalByCode(refusal: ServerRefusal | null | undefined): string | undefined {
  const code = refusal?.code
  if (!code) return undefined
  if (FAMILY_CODES.has(code) && refusal?.field) return undefined
  const key = `errors:api.${code}`
  return i18n.exists(key) ? i18n.t(key) : undefined
}

/**
 * A refusal as a form shows it at its field: what is wrong, then what to do —
 * in the console's language when the code is known, in the API's own words
 * when it is not.
 */
export function refusalMessage(refusal: ServerRefusal): string {
  return (
    refusalByCode(refusal) ??
    ([refusal.detail, refusal.hint].filter(Boolean).join(' ') || i18n.t('errors:refused'))
  )
}

/** What the refusal behind `error` says about the request member `field`, if it names it. */
export function refusalAt(error: unknown, field: string): string | undefined {
  const refusal = serverRefusal(error)
  return refusal?.field === field ? refusalMessage(refusal) : undefined
}

/**
 * The words for a failed request, in the console's language: a translation
 * the page keeps for this code (`overrides` is a key prefix such as
 * `settings:deleteRefusals`, for refusals a page words for its own action),
 * else the translation of the code, else the page's own translated fallback
 * (`fallbackKey`, `namespace:key`), and only without all of those the API's
 * `detail`.
 */
export function apiErrorMessage(error: unknown, fallbackKey?: string, overrides?: string): string {
  const refusal = serverRefusal(error)
  const own = overrides && refusal?.code && i18n.exists(`${overrides}.${refusal.code}`)
    ? i18n.t(`${overrides}.${refusal.code}`)
    : undefined
  return (
    own ??
    refusalByCode(refusal) ??
    (fallbackKey ? i18n.t(fallbackKey) : undefined) ??
    refusal?.detail ??
    i18n.t('errors:generic')
  )
}
