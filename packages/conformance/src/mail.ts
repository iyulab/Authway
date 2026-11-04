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

/**
 * Decodes quoted-printable soft line breaks and =XX escapes. The escapes are
 * bytes, so the text is read as UTF-8 once they are bytes again: a character
 * outside ASCII is several of them.
 */
function decodeQuotedPrintable(input: string): string {
  const text = input.replace(/=\r?\n/g, '')
  const bytes: number[] = []
  for (let i = 0; i < text.length; i++) {
    const hex = text[i] === '=' ? text.slice(i + 1, i + 3) : ''
    if (/^[0-9A-F]{2}$/i.test(hex)) {
      bytes.push(parseInt(hex, 16))
      i += 2
    } else {
      bytes.push(...Buffer.from(text[i], 'utf8'))
    }
  }
  return Buffer.from(bytes).toString('utf8')
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

/**
 * The message's text as a mail client shows it: each part decoded, or the
 * single body when the message has no parts. A multipart message's whole
 * content is the parts still encoded — reading links from it would take a
 * quoted-printable `=3D` for part of a token.
 */
function bodies(message: MailHogMessage): string[] {
  const parts = message.MIME?.Parts ?? []
  return (parts.length > 0 ? parts : [message.Content]).map(decodePart)
}

/**
 * Waits for a message addressed to `recipient` and returns the first link in it
 * whose path ends with `path` (e.g. "/invitation/accept") and that `accept`
 * takes — newest message first.
 */
export async function waitForLink(
  mailApi: string,
  recipient: string,
  path: string,
  timeoutMs = 15_000,
  accept: (url: URL) => boolean = () => true,
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
            if (url.pathname.endsWith(path) && accept(url)) return url
          }
        }
      }
    }
    await new Promise((r) => setTimeout(r, 500))
  }
  throw new Error(`No mail to ${recipient} containing a ${path} link within ${timeoutMs}ms`)
}

/**
 * Waits for a message addressed to `recipient` that `accept` takes (its decoded
 * text, every part joined) and returns that text — newest message first.
 */
export async function waitForMessage(
  mailApi: string,
  recipient: string,
  accept: (text: string) => boolean = () => true,
  timeoutMs = 15_000,
): Promise<string> {
  const deadline = Date.now() + timeoutMs
  const query = `${mailApi}/api/v2/search?kind=to&query=${encodeURIComponent(recipient)}`
  while (Date.now() < deadline) {
    const res = await fetch(query)
    if (res.ok) {
      const { items } = (await res.json()) as { items: MailHogMessage[] }
      for (const message of items) {
        const text = bodies(message).join('\n')
        if (accept(text)) return text
      }
    }
    await new Promise((r) => setTimeout(r, 500))
  }
  throw new Error(`No mail to ${recipient} that the scenario takes within ${timeoutMs}ms`)
}

/** Whether a text is written in Korean: it carries Hangul, which no English string of Authway's does. */
export const isKorean = (text: string): boolean => /[가-힣]/.test(text)
