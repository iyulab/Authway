import { describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { isKorean, waitForMessage } from '../src/mail.js'
import { randomToken } from '../src/pkce.js'
import { Provider } from '../src/provider.js'

const provider = new Provider(loadConfig())

// The one who invites knows the language of the one invited — Authway knows
// neither, and a server calling for an application sends no Accept-Language.
// The call can name the language; a language there is no email in is refused.
describe('admin API: the language of an invitation email', () => {
  const address = () => `conformance-${randomToken(6)}@example.test`

  const invite = (body: Record<string, unknown>, acceptLanguage?: string) =>
    provider.admin('/api/v1/invitations', {
      method: 'POST',
      body: JSON.stringify(body),
      headers: acceptLanguage ? { 'Accept-Language': acceptLanguage } : undefined,
    })

  const mailedInKorean = async (email: string) => isKorean(await waitForMessage(provider.config.mailApi, email))

  it('writes the email in the language the call names, whatever Accept-Language says', async () => {
    const named = address()
    const res = await invite({ email: named, locale: 'ko' })
    await conform('POST', '/api/v1/invitations', res)
    expect(res.status).toBe(201)
    expect(await mailedInKorean(named)).toBe(true)

    const overruled = address()
    const english = await invite({ email: overruled, locale: 'en' }, 'ko-KR,ko;q=0.9')
    await conform('POST', '/api/v1/invitations', english)
    expect(english.status).toBe(201)
    expect(await mailedInKorean(overruled)).toBe(false)
  })

  it('follows Accept-Language when the call names none, and is English when that names nothing either', async () => {
    const asked = address()
    expect((await invite({ email: asked }, 'ko-KR,ko;q=0.9')).status).toBe(201)
    expect(await mailedInKorean(asked)).toBe(true)

    const unasked = address()
    expect((await invite({ email: unasked }, 'fr-FR,fr;q=0.9')).status).toBe(201)
    expect(await mailedInKorean(unasked)).toBe(false)
  })

  it('refuses a language there is no email in, creating nothing', async () => {
    const email = address()
    for (const locale of ['fr', 'ko, en', '*']) {
      const res = await invite({ email, locale })
      await conform('POST', '/api/v1/invitations', res)
      expect(res.status, JSON.stringify(locale)).toBe(400)
      expect(await res.json(), JSON.stringify(locale)).toMatchObject({ code: 'invalid_request', field: 'locale' })
    }
    // Not a string at all is a malformed body: refused, though the member it names is not promised.
    const malformed = await invite({ email, locale: 7 })
    expect(malformed.status).toBe(400)
    expect(await malformed.json()).toMatchObject({ code: 'invalid_request' })
    // Nothing was created by the refusals: the same address can still be invited.
    const created = await invite({ email, locale: 'en' })
    await conform('POST', '/api/v1/invitations', created)
    expect(created.status).toBe(201)
  })

  it('lets a resend name the language of the new email, and refuses one it has no email in', async () => {
    const email = address()
    const created = await invite({ email, locale: 'en' })
    expect(created.status).toBe(201)
    const { invitation } = (await created.json()) as { invitation: { id: string } }
    expect(await mailedInKorean(email)).toBe(false)

    const resend = await provider.admin(`/api/v1/invitations/${invitation.id}/resend`, {
      method: 'POST',
      body: JSON.stringify({ locale: 'ko' }),
    })
    await conform('POST', '/api/v1/invitations/{id}/resend', resend)
    expect(resend.status).toBe(200)
    // A Korean message among the mail to this address: the resend's.
    await waitForMessage(provider.config.mailApi, email, isKorean)

    const refused = await provider.admin(`/api/v1/invitations/${invitation.id}/resend`, {
      method: 'POST',
      body: JSON.stringify({ locale: 'fr' }),
    })
    await conform('POST', '/api/v1/invitations/{id}/resend', refused)
    expect(refused.status).toBe(400)
    expect(await refused.json()).toMatchObject({ code: 'invalid_request', field: 'locale' })
  })
})
