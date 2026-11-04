import { describe, it, expect } from 'vitest'
import { SUPPORTED_LANGUAGES } from './index'

// locales/<language>/<namespace>.json — every language has every namespace,
// and every namespace has the same keys in each language.
const files = import.meta.glob<Record<string, unknown>>('./locales/*/*.json', { eager: true, import: 'default' })

type Tree = Record<string, unknown>
const byLanguage: Record<string, Record<string, Tree>> = {}
for (const [path, tree] of Object.entries(files)) {
  const [, language, namespace] = /\.\/locales\/([^/]+)\/([^/]+)\.json$/.exec(path)!
  ;(byLanguage[language] ??= {})[namespace] = tree as Tree
}

const flatten = (tree: Tree, prefix = ''): Record<string, string> =>
  Object.entries(tree).reduce<Record<string, string>>((all, [key, value]) => {
    const path = prefix ? `${prefix}.${key}` : key
    return typeof value === 'string' ? { ...all, [path]: value } : { ...all, ...flatten(value as Tree, path) }
  }, {})

const namespaces = Object.keys(byLanguage.en)

// Words that read the same in both languages: brand and product names, and
// identifiers (an address, a standard's number). Anything else that is
// identical in ko and en was probably left untranslated.
const SAME_IN_BOTH = new Set([
  'common.brand',
  'dashboard.system.api',
  'webhooks.columns.url',
  'webhooks.form.url',
  'webhooks.form.urlPlaceholder',
  'webhooks.toast.httpStatus',
  'tenants.create.slugPlaceholder',
  'users.provider.google',
  'users.provider.github',
  'users.provider.microsoft',
  'users.provider.apple',
  'clients.form.providers.google',
  'clients.form.providers.github',
  'clients.form.providers.microsoft',
  'clients.form.providers.apple',
  'clients.form.scopes.openid',
  'clients.form.homePagePlaceholder',
  'clients.form.logoPlaceholder',
  'clients.form.initiateLoginPlaceholder',
  'clients.form.redirectUrisPlaceholder',
  'clients.form.postLogoutPlaceholder',
  'clients.form.originsPlaceholder',
  'clients.form.audiencePlaceholder',
  'clients.form.backchannelPlaceholder',
  'settings.sections.capabilities.deviceHelp',
  'settings.form.domainsPlaceholder',
  'invitations.form.emailPlaceholder',
  'common.layout.github',
])

const placeholders = (text: string) => [...text.matchAll(/\{\{\s*(\w+)[^}]*\}\}/g)].map((m) => m[1]).sort()
const tags = (text: string) => [...text.matchAll(/<\/?(\w+)>/g)].map((m) => m[1]).sort()

describe('the console’s languages', () => {
  it('are Korean and English', () => {
    expect(SUPPORTED_LANGUAGES.map((l) => l.code)).toEqual(['ko', 'en'])
    expect(Object.keys(byLanguage).sort()).toEqual(['en', 'ko'])
  })

  it('have the same namespaces', () => {
    expect(Object.keys(byLanguage.ko).sort()).toEqual(Object.keys(byLanguage.en).sort())
  })

  describe.each(namespaces)('namespace %s', (namespace) => {
    const en = flatten(byLanguage.en[namespace])
    const ko = flatten(byLanguage.ko[namespace])

    it('has every English key in Korean, and no more', () => {
      expect(Object.keys(ko).sort()).toEqual(Object.keys(en).sort())
    })

    it('has no empty text', () => {
      for (const [key, value] of [...Object.entries(en), ...Object.entries(ko)]) {
        expect(value.trim(), `${namespace}.${key}`).not.toBe('')
      }
    })

    it('leaves nothing untranslated', () => {
      const same = Object.keys(en).filter(
        (key) => en[key] === ko[key] && /[A-Za-z]{2,}/.test(en[key]) && !SAME_IN_BOTH.has(`${namespace}.${key}`),
      )
      expect(same).toEqual([])
    })

    it('uses the same placeholders and markup in both languages', () => {
      for (const key of Object.keys(en)) {
        expect(placeholders(ko[key] ?? ''), `${namespace}.${key} placeholders`).toEqual(placeholders(en[key]))
        expect(tags(ko[key] ?? ''), `${namespace}.${key} markup`).toEqual(tags(en[key]))
      }
    })

    it('has Korean text in Korean', () => {
      const withoutHangul = Object.keys(ko).filter(
        (key) => !/[가-힣]/.test(ko[key]) && /[A-Za-z]{2,}/.test(ko[key]) && !SAME_IN_BOTH.has(`${namespace}.${key}`),
      )
      expect(withoutHangul).toEqual([])
    })
  })
})
