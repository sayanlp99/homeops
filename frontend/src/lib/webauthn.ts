import { post } from './api'

// The server speaks the WebAuthn JSON format (base64url binary fields). Newer browsers can
// convert natively; these helpers cover older Safari/Chrome versions.

const b64urlToBuf = (s: string) => {
  const pad = '='.repeat((4 - (s.length % 4)) % 4)
  const bin = atob((s + pad).replace(/-/g, '+').replace(/_/g, '/'))
  return Uint8Array.from(bin, (c) => c.charCodeAt(0)).buffer
}

const bufToB64url = (b: ArrayBuffer | null | undefined) => {
  if (!b) return undefined
  let s = ''
  for (const byte of new Uint8Array(b)) s += String.fromCharCode(byte)
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

type Json = Record<string, any> // eslint-disable-line @typescript-eslint/no-explicit-any

function toCreationOptions(o: Json): PublicKeyCredentialCreationOptions {
  return {
    ...o,
    challenge: b64urlToBuf(o.challenge),
    user: { ...o.user, id: b64urlToBuf(o.user.id) },
    excludeCredentials: (o.excludeCredentials ?? []).map((c: Json) => ({ ...c, id: b64urlToBuf(c.id) })),
  } as PublicKeyCredentialCreationOptions
}

function toRequestOptions(o: Json): PublicKeyCredentialRequestOptions {
  return {
    ...o,
    challenge: b64urlToBuf(o.challenge),
    allowCredentials: (o.allowCredentials ?? []).map((c: Json) => ({ ...c, id: b64urlToBuf(c.id) })),
  } as PublicKeyCredentialRequestOptions
}

function credentialToJSON(c: PublicKeyCredential): Json {
  if (typeof c.toJSON === 'function') return c.toJSON() as Json
  const r = c.response as AuthenticatorAttestationResponse & AuthenticatorAssertionResponse
  return {
    id: c.id,
    rawId: bufToB64url(c.rawId),
    type: c.type,
    authenticatorAttachment: c.authenticatorAttachment,
    clientExtensionResults: c.getClientExtensionResults(),
    response: {
      clientDataJSON: bufToB64url(r.clientDataJSON),
      attestationObject: bufToB64url(r.attestationObject),
      transports: r.getTransports?.(),
      authenticatorData: bufToB64url(r.authenticatorData),
      signature: bufToB64url(r.signature),
      userHandle: bufToB64url(r.userHandle),
    },
  }
}

export const passkeysSupported = () => typeof window.PublicKeyCredential !== 'undefined'

export async function registerPasskey(name?: string) {
  const { publicKey } = await post<{ publicKey: Json }>('/me/passkeys/begin')
  const cred = (await navigator.credentials.create({ publicKey: toCreationOptions(publicKey) })) as PublicKeyCredential | null
  if (!cred) throw new Error('Passkey setup was cancelled')
  await post(`/me/passkeys/finish${name ? `?name=${encodeURIComponent(name)}` : ''}`, credentialToJSON(cred))
}

export async function loginWithPasskey() {
  const { ceremonyId, options } = await post<{ ceremonyId: string; options: { publicKey: Json } }>('/auth/passkey/login/begin')
  const cred = (await navigator.credentials.get({ publicKey: toRequestOptions(options.publicKey) })) as PublicKeyCredential | null
  if (!cred) throw new Error('Sign-in was cancelled')
  await post(`/auth/passkey/login/finish?ceremony=${encodeURIComponent(ceremonyId)}`, credentialToJSON(cred))
}
