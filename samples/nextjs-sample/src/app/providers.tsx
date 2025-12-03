'use client'

import { ReactNode } from 'react'
import { AuthwayProvider } from '@authway/react'
import { useRouter } from 'next/navigation'

const authConfig = {
  domain: process.env.NEXT_PUBLIC_AUTHWAY_DOMAIN || 'http://localhost:8081',
  clientId: process.env.NEXT_PUBLIC_AUTHWAY_CLIENT_ID || 'nextjs-sample-client',
  redirectUri: typeof window !== 'undefined' ? window.location.origin : 'http://localhost:3100',
}

interface ProvidersProps {
  children: ReactNode
}

export function Providers({ children }: ProvidersProps) {
  const router = useRouter()

  return (
    <AuthwayProvider
      config={authConfig}
      onRedirectCallback={(appState) => {
        // Navigate to the intended destination after login
        router.replace(appState?.returnTo || '/')
      }}
    >
      {children}
    </AuthwayProvider>
  )
}
