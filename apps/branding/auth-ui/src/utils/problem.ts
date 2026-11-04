import i18n from '../i18n'

/** The members of a refusal (RFC 9457 problem details) a screen reads. */
export interface Problem {
  code?: string
  detail?: string
}

/**
 * The text to show for a refused request, in the language of the screen.
 *
 * The server words a refusal in one language only, and a person reads the
 * screen's language, so the refusal's `code` — the stable identifier — picks
 * the text: its translation when there is one, otherwise the screen's own
 * translated `fallback`. The server's `detail` is never shown; it is for the
 * developer reading the response.
 */
export function problemMessage(problem: Problem | null | undefined, fallback: string): string {
  const key = problem?.code ? `errors:api.${problem.code}` : undefined
  return key && i18n.exists(key) ? i18n.t(key) : fallback
}
