/**
 * The browser's half of a passkey ceremony.
 *
 * The server speaks the JSON forms of the WebAuthn specification
 * (PublicKeyCredentialCreationOptionsJSON, PublicKeyCredentialRequestOptionsJSON
 * and the registration and authentication response JSON); the browser's
 * credential API takes buffers. This converts between them, so the screen and
 * the server only ever pass the server's own JSON on. It does not use
 * PublicKeyCredential.parse...FromJSON() and toJSON(): browsers that can do a
 * passkey but not those would otherwise be left out, and the conversion is the
 * same few lines either way.
 */

/** A passkey ceremony the user ended - closed the prompt, chose no passkey. */
export class PasskeyCancelled extends Error {
  constructor() {
    super('passkey cancelled')
    this.name = 'PasskeyCancelled'
  }
}

/**
 * The device already holds a passkey for this account: the server named the
 * ones it knows in excludeCredentials, and the browser refuses to make a second
 * (InvalidStateError). Not a failure - the user has what they wanted.
 */
export class PasskeyExists extends Error {
  constructor() {
    super('passkey exists')
    this.name = 'PasskeyExists'
  }
}

/** Whether this browser can make and use passkeys at all. */
export function passkeysSupported(): boolean {
  return typeof window !== 'undefined' && typeof window.PublicKeyCredential === 'function' && !!navigator.credentials
}

/** Whether the browser can offer passkeys in a text field's autofill. */
export async function conditionalMediationAvailable(): Promise<boolean> {
  if (!passkeysSupported()) return false
  const available = (window.PublicKeyCredential as unknown as { isConditionalMediationAvailable?: () => Promise<boolean> })
    .isConditionalMediationAvailable
  if (typeof available !== 'function') return false
  try {
    return await available.call(window.PublicKeyCredential)
  } catch {
    return false
  }
}

/** base64url, as WebAuthn JSON writes buffers: no padding, - and _ for + and /. */
export function fromBase64Url(text: string): ArrayBuffer {
  const base64 = text.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(text.length / 4) * 4, '=')
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes.buffer
}

export function toBase64Url(buffer: ArrayBuffer | null | undefined): string | undefined {
  if (!buffer) return undefined
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

interface CredentialDescriptorJSON {
  id: string
  type: 'public-key'
  transports?: AuthenticatorTransport[]
}

export interface CreationOptionsJSON {
  challenge: string
  user: { id: string; name: string; displayName: string }
  excludeCredentials?: CredentialDescriptorJSON[]
  [member: string]: unknown
}

export interface RequestOptionsJSON {
  challenge: string
  allowCredentials?: CredentialDescriptorJSON[]
  [member: string]: unknown
}

function descriptors(list?: CredentialDescriptorJSON[]): PublicKeyCredentialDescriptor[] | undefined {
  return list?.map((d) => ({ ...d, id: fromBase64Url(d.id) }))
}

/** The browser's input for navigator.credentials.create. */
export function creationOptions(json: CreationOptionsJSON): PublicKeyCredentialCreationOptions {
  return {
    ...(json as unknown as PublicKeyCredentialCreationOptions),
    challenge: fromBase64Url(json.challenge),
    user: { ...json.user, id: fromBase64Url(json.user.id) },
    excludeCredentials: descriptors(json.excludeCredentials),
  }
}

/** The browser's input for navigator.credentials.get. */
export function requestOptions(json: RequestOptionsJSON): PublicKeyCredentialRequestOptions {
  return {
    ...(json as unknown as PublicKeyCredentialRequestOptions),
    challenge: fromBase64Url(json.challenge),
    allowCredentials: descriptors(json.allowCredentials),
  }
}

/** A user closing the prompt is the browser's NotAllowedError (or an abort). */
function isCancel(error: unknown): boolean {
  return error instanceof DOMException && (error.name === 'NotAllowedError' || error.name === 'AbortError')
}

/** A new passkey as the server takes it: PublicKeyCredential.toJSON(). */
export async function createPasskey(json: CreationOptionsJSON, signal?: AbortSignal): Promise<Record<string, unknown>> {
  let credential: Credential | null
  try {
    credential = await navigator.credentials.create({ publicKey: creationOptions(json), signal })
  } catch (error) {
    if (isCancel(error)) throw new PasskeyCancelled()
    if (error instanceof DOMException && error.name === 'InvalidStateError') throw new PasskeyExists()
    throw error
  }
  if (!credential) throw new PasskeyCancelled()
  const created = credential as PublicKeyCredential
  const response = created.response as AuthenticatorAttestationResponse
  return {
    id: created.id,
    rawId: toBase64Url(created.rawId),
    type: created.type,
    authenticatorAttachment: created.authenticatorAttachment ?? undefined,
    response: {
      clientDataJSON: toBase64Url(response.clientDataJSON),
      attestationObject: toBase64Url(response.attestationObject),
      transports: response.getTransports?.() ?? [],
    },
    clientExtensionResults: created.getClientExtensionResults(),
  }
}

/**
 * A passkey's answer to a sign-in, as the server takes it:
 * PublicKeyCredential.toJSON(). conditional offers the passkeys in the
 * autofill of a field marked autocomplete="... webauthn" and waits there until
 * the user picks one (or signal ends it).
 */
export async function getPasskey(
  json: RequestOptionsJSON,
  { conditional = false, signal }: { conditional?: boolean; signal?: AbortSignal } = {},
): Promise<Record<string, unknown>> {
  let credential: Credential | null
  try {
    const request: CredentialRequestOptions = { publicKey: requestOptions(json), signal }
    if (conditional) (request as { mediation?: string }).mediation = 'conditional'
    credential = await navigator.credentials.get(request)
  } catch (error) {
    if (isCancel(error)) throw new PasskeyCancelled()
    throw error
  }
  if (!credential) throw new PasskeyCancelled()
  const got = credential as PublicKeyCredential
  const response = got.response as AuthenticatorAssertionResponse
  return {
    id: got.id,
    rawId: toBase64Url(got.rawId),
    type: got.type,
    authenticatorAttachment: got.authenticatorAttachment ?? undefined,
    response: {
      clientDataJSON: toBase64Url(response.clientDataJSON),
      authenticatorData: toBase64Url(response.authenticatorData),
      signature: toBase64Url(response.signature),
      userHandle: toBase64Url(response.userHandle),
    },
    clientExtensionResults: got.getClientExtensionResults(),
  }
}
