import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  createPasskey,
  creationOptions,
  fromBase64Url,
  getPasskey,
  PasskeyCancelled,
  PasskeyExists,
  requestOptions,
  toBase64Url,
} from './passkey'

const bytes = (...values: number[]) => new Uint8Array(values).buffer

describe('base64url', () => {
  it('round trips what WebAuthn JSON carries, without padding', () => {
    // 0xfb 0xff 0xfe uses + and / in base64; the JSON form writes - and _.
    const buffer = bytes(0xfb, 0xff, 0xfe, 1)
    const text = toBase64Url(buffer)
    expect(text).toBe('-__-AQ')
    expect(new Uint8Array(fromBase64Url(text!))).toEqual(new Uint8Array(buffer))
  })

  it('has nothing to say for an absent buffer', () => {
    expect(toBase64Url(null)).toBeUndefined()
    expect(toBase64Url(undefined)).toBeUndefined()
  })
})

describe('options', () => {
  it('turns the JSON the server sent into the buffers the browser takes, keeping the rest', () => {
    const options = creationOptions({
      challenge: 'AQID',
      rp: { id: 'auth.example.com', name: 'Authway' },
      user: { id: 'BAUG', name: 'pat@example.com', displayName: 'pat@example.com · Acme' },
      excludeCredentials: [{ id: 'BwgJ', type: 'public-key', transports: ['internal'] }],
      authenticatorSelection: { residentKey: 'required', userVerification: 'required' },
    })
    expect(new Uint8Array(options.challenge as ArrayBuffer)).toEqual(new Uint8Array([1, 2, 3]))
    expect(new Uint8Array(options.user.id as ArrayBuffer)).toEqual(new Uint8Array([4, 5, 6]))
    expect(options.user.displayName).toBe('pat@example.com · Acme')
    expect(new Uint8Array(options.excludeCredentials![0].id as ArrayBuffer)).toEqual(new Uint8Array([7, 8, 9]))
    expect(options.excludeCredentials![0].transports).toEqual(['internal'])
    expect(options.authenticatorSelection).toEqual({ residentKey: 'required', userVerification: 'required' })
    expect(options.rp).toEqual({ id: 'auth.example.com', name: 'Authway' })
  })

  it('names no credential for a sign-in with nobody named', () => {
    const options = requestOptions({ challenge: 'AQID', rpId: 'auth.example.com', userVerification: 'required' })
    expect(new Uint8Array(options.challenge as ArrayBuffer)).toEqual(new Uint8Array([1, 2, 3]))
    expect(options.allowCredentials).toBeUndefined()
    expect(options.rpId).toBe('auth.example.com')
  })
})

describe('the ceremonies', () => {
  const credentials = { create: vi.fn(), get: vi.fn() }
  Object.defineProperty(navigator, 'credentials', { value: credentials, configurable: true })
  afterEach(() => vi.clearAllMocks())

  const newCredential = {
    id: 'AQID',
    rawId: bytes(1, 2, 3),
    type: 'public-key',
    authenticatorAttachment: 'platform',
    response: {
      clientDataJSON: bytes(9, 9),
      attestationObject: bytes(8, 8),
      getTransports: () => ['internal', 'hybrid'],
    },
    getClientExtensionResults: () => ({ credProps: { rk: true } }),
  }

  it('writes a new passkey as the server takes it', async () => {
    credentials.create.mockResolvedValue(newCredential)
    const json = await createPasskey({ challenge: 'AQID', user: { id: 'BAUG', name: 'a', displayName: 'a' } })
    expect(credentials.create.mock.calls[0][0].publicKey.challenge).toBeInstanceOf(ArrayBuffer)
    expect(json).toEqual({
      id: 'AQID',
      rawId: 'AQID',
      type: 'public-key',
      authenticatorAttachment: 'platform',
      response: { clientDataJSON: 'CQk', attestationObject: 'CAg', transports: ['internal', 'hybrid'] },
      clientExtensionResults: { credProps: { rk: true } },
    })
  })

  it('writes a sign-in answer with the user handle', async () => {
    credentials.get.mockResolvedValue({
      id: 'AQID',
      rawId: bytes(1, 2, 3),
      type: 'public-key',
      authenticatorAttachment: 'platform',
      response: { clientDataJSON: bytes(9), authenticatorData: bytes(7), signature: bytes(6), userHandle: bytes(5) },
      getClientExtensionResults: () => ({}),
    })
    const json = await getPasskey({ challenge: 'AQID' })
    expect(json.response).toEqual({ clientDataJSON: 'CQ', authenticatorData: 'Bw', signature: 'Bg', userHandle: 'BQ' })
    expect(credentials.get.mock.calls[0][0].mediation).toBeUndefined()
  })

  it('waits behind the field for a conditional sign-in, and can be stopped', async () => {
    credentials.get.mockResolvedValue(null)
    const controller = new AbortController()
    await expect(getPasskey({ challenge: 'AQID' }, { conditional: true, signal: controller.signal })).rejects.toBeInstanceOf(PasskeyCancelled)
    const request = credentials.get.mock.calls[0][0]
    expect(request.mediation).toBe('conditional')
    expect(request.signal).toBe(controller.signal)
  })

  it('takes the user closing the prompt, or ending the request, as cancelling and nothing else', async () => {
    credentials.create.mockRejectedValue(new DOMException('closed', 'NotAllowedError'))
    await expect(createPasskey({ challenge: 'AQID', user: { id: 'BAUG', name: 'a', displayName: 'a' } })).rejects.toBeInstanceOf(PasskeyCancelled)
    credentials.get.mockRejectedValue(new DOMException('stopped', 'AbortError'))
    await expect(getPasskey({ challenge: 'AQID' })).rejects.toBeInstanceOf(PasskeyCancelled)

    // A device that already holds a passkey for the account is told apart: the user has what they wanted.
    credentials.create.mockRejectedValue(new DOMException('exists', 'InvalidStateError'))
    await expect(createPasskey({ challenge: 'AQID', user: { id: 'BAUG', name: 'a', displayName: 'a' } })).rejects.toBeInstanceOf(PasskeyExists)

    // Anything else is a failure to report, not a choice.
    const broken = new DOMException('no such device', 'NotSupportedError')
    credentials.get.mockRejectedValue(broken)
    await expect(getPasskey({ challenge: 'AQID' })).rejects.toBe(broken)
  })
})
