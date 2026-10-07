/**
 * Reads mail captured by a MailHog-compatible SMTP sink.
 */
interface MailHogPart {
  Headers?: Record<string, string[]>
  Body: string
}

interface MailHogMessage {
  Content: MailHogPart
  MIME?: { Parts?: MailHogPart[] } | null
}

/** Decodes quoted-printable soft line breaks and =XX escapes. */
function decodeQuotedPrintable(input: string): string {
  return input
    .replace(/=\r?\n/g, '')
    .replace(/=([0-9A-F]{2})/gi, (_, hex: string) => String.fromCharCode(parseInt(hex, 16)))
}

/**
 * Decodes a part according to its own Content-Transfer-Encoding. Decoding a
 * part that is not quoted-printable would corrupt any "=XX" it contains — a
 * token that happens to begin with two hex digits right after "token=".
 */
function decodePart(part: MailHogPart): string {
  const encoding = Object.entries(part.Headers ?? {})
    .find(([name]) => name.toLowerCase() === 'content-transfer-encoding')?.[1]?.[0]
    ?.trim()
    .toLowerCase()
  if (encoding === 'quoted-printable') return decodeQuotedPrintable(part.Body)
  if (encoding === 'base64') return Buffer.from(part.Body.replace(/\s+/g, ''), 'base64').toString('utf8')
  return part.Body
}

function bodies(message: MailHogMessage): string[] {
  return [message.Content, ...(message.MIME?.Parts ?? [])].map(decodePart)
}

/**
 * Waits for a message addressed to `recipient` and returns the first link in it
 * whose path ends with `path` (e.g. "/invitation/accept").
 */
export async function waitForLink(
  mailApi: string,
  recipient: string,
  path: string,
  timeoutMs = 15_000,
): Promise<URL> {
  const deadline = Date.now() + timeoutMs
  const query = `${mailApi}/api/v2/search?kind=to&query=${encodeURIComponent(recipient)}`
  while (Date.now() < deadline) {
    const res = await fetch(query)
    if (res.ok) {
      const { items } = (await res.json()) as { items: MailHogMessage[] }
      for (const message of items) {
        for (const body of bodies(message)) {
          for (const match of body.matchAll(/https?:\/\/[^\s"'<>]+/g)) {
            const url = new URL(match[0].replace(/&amp;/g, '&'))
            if (url.pathname.endsWith(path)) return url
          }
        }
      }
    }
    await new Promise((r) => setTimeout(r, 500))
  }
  throw new Error(`No mail to ${recipient} containing a ${path} link within ${timeoutMs}ms`)
}
