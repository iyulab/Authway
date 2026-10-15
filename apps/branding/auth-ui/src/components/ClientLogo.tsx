import React, { useState } from 'react'

/** Whether a logo address may be shown: an absolute https URL, which is all the server accepts. */
export function showableLogo(uri: string | undefined): string | undefined {
  if (!uri) return undefined
  try {
    return new URL(uri).protocol === 'https:' ? uri : undefined
  } catch {
    return undefined
  }
}

/**
 * The application's logo, as its registration names it (logo_uri), above the
 * sentence that names the application. Nothing when there is none or it does
 * not load — the name says the same thing.
 */
const ClientLogo: React.FC<{ uri?: string; name?: string }> = ({ uri, name }) => {
  const [failed, setFailed] = useState(false)
  const src = showableLogo(uri)
  if (!src || failed) return null
  return (
    <img
      src={src}
      alt={name ?? ''}
      referrerPolicy="no-referrer"
      className="mx-auto mb-2 h-12 max-w-[12rem] object-contain"
      onError={() => setFailed(true)}
    />
  )
}

export default ClientLogo
