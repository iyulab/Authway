import { beforeEach } from 'vitest'
import i18n from '@/i18n'

/** Runs the tests of the enclosing describe/file in `language` (they start in English). */
export function inLanguage(language: 'ko' | 'en') {
  beforeEach(async () => {
    await i18n.changeLanguage(language)
  })
}
