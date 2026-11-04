import type { Locale } from '@authway/react'

// The words this sample puts on screen, in the two languages Authway
// supports. An application with more text would use an i18n library; a table
// keeps the sample small.
export const messages = {
  en: {
    home: 'Home',
    callTheApi: 'Call the API',
    loading: 'Loading…',
    signedOut: 'Signed out',
    signedIn: 'Signed in',
    signIn: 'Sign in',
    signOut: 'Sign out',
    signInFailed: 'Sign-in failed',
    theApi: 'The API',
    setClientId: 'Set VITE_AUTHWAY_CLIENT_ID in .env.local — see the README.'
  },
  ko: {
    home: '홈',
    callTheApi: 'API 호출',
    loading: '불러오는 중…',
    signedOut: '로그아웃 상태',
    signedIn: '로그인 상태',
    signIn: '로그인',
    signOut: '로그아웃',
    signInFailed: '로그인에 실패했습니다',
    theApi: 'API',
    setClientId: '.env.local 에 VITE_AUTHWAY_CLIENT_ID 를 설정하세요 — README 를 참고하세요.'
  }
} as const satisfies Record<Locale, Record<string, string>>

export type Messages = Record<keyof (typeof messages)['en'], string>
