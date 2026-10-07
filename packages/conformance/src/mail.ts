/**
 * Reads mail captured by a MailHog-compatible SMTP sink.
 */
interface MailHogMessage {
  Content: { Headers: Record<string, string[]>; Body: string }
  MIME?: { Parts?: { Body: string }[] } | null
}

/** Decodes quoted-printable soft line breaks and =XX escapes. */
function decodeQuotedPrintable(input: string): string {
  return input
    .replace(/=\r?\n/g, '')
    .replace(/=([0-9A-F]{2})/gi, (_, hex: string) => String.fromCharCode(parseInt(hex, 16)))
}

function bodies(message: MailHogMessage): string[] {
  const parts = message.MIME?.Parts?.map((p) => p.Body) ?? []
  return [message.Content.Body, ...parts].map(decodeQuotedPrintable)
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
