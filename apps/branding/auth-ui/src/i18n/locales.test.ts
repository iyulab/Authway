import { describe, expect, it } from 'vitest'
const files = import.meta.glob('./locales/*/*.json', { eager: true, import: 'default' }) as Record<string, Tree>
const ko = Object.fromEntries(Object.entries(files).filter(([p]) => p.includes('/ko/')).map(([p, v]) => [p.split('/').pop()!.replace('.json', ''), v]))
const en = Object.fromEntries(Object.entries(files).filter(([p]) => p.includes('/en/')).map(([p, v]) => [p.split('/').pop()!.replace('.json', ''), v]))

/**
 * The two languages stay in step: a key in one is in the other, a screen only
 * asks for keys that exist, and no call hides a missing key behind a default
 * (an English default shows in the Korean screen, and nothing notices).
 */

type Tree = { [key: string]: string | Tree }

const NAMESPACES = ['common', 'auth', 'consent', 'password', 'errors'] as const

function flat(tree: Tree, prefix = ''): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [key, value] of Object.entries(tree)) {
    if (typeof value === 'string') out[prefix + key] = value
    else Object.assign(out, flat(value, `${prefix}${key}.`))
  }
  return out
}

const tables = {
  ko: Object.fromEntries(NAMESPACES.map((ns) => [ns, flat(ko[ns])])),
  en: Object.fromEntries(NAMESPACES.map((ns) => [ns, flat(en[ns])])),
}

// i18next plural forms: "on_one", "on_other" are asked for as "on".
const PLURAL = /_(zero|one|two|few|many|other)$/
const asked = (table: Record<string, string>, key: string) =>
  key in table || Object.keys(table).some((k) => k.replace(PLURAL, '') === key)

describe('locales', () => {
  for (const ns of NAMESPACES) {
    it(`${ns}: ko and en have the same keys`, () => {
      const koKeys = Object.keys(tables.ko[ns]).map((k) => k.replace(PLURAL, ''))
      const enKeys = Object.keys(tables.en[ns]).map((k) => k.replace(PLURAL, ''))
      expect([...new Set(koKeys)].sort()).toEqual([...new Set(enKeys)].sort())
    })

    it(`${ns}: no ko text is left as the English text`, () => {
      const same = Object.entries(tables.ko[ns]).filter(
        ([key, value]) => value === tables.en[ns][key] && /[A-Za-z]{4,}/.test(value) && !value.startsWith('$t('),
      )
      // Names and product terms are the same in both languages.
      const allowed = /^(Authway|Google|GitHub|Microsoft|Apple|OAuth|OpenID|JWT|ABCD-EFGH)\b/
      expect(same.filter(([, value]) => !allowed.test(value)).map(([key]) => `${ns}:${key}`)).toEqual([])
    })
  }

  const sources = import.meta.glob(['../**/*.ts', '../**/*.tsx', '!../**/*.test.*', '!../test/**'], {
    query: '?raw',
    import: 'default',
    eager: true,
  }) as Record<string, string>

  // t('ns:key') or t("key", ...) — the key as written, nothing computed.
  const CALL = /\bt\(\s*(['"])([A-Za-z]+:[A-Za-z0-9_.]+)\1(\s*,\s*(['"`]))?/g

  it('every key a screen asks for exists in both languages', () => {
    const missing: string[] = []
    for (const [file, text] of Object.entries(sources)) {
      for (const match of text.matchAll(CALL)) {
        const [ns, key] = match[2].split(':') as [(typeof NAMESPACES)[number], string]
        for (const lang of ['ko', 'en'] as const) {
          if (!tables[lang][ns] || !asked(tables[lang][ns], key)) missing.push(`${lang} ${match[2]} (${file})`)
        }
      }
    }
    expect(missing).toEqual([])
  })

  it('no call hides a missing key behind a default text', () => {
    const hidden: string[] = []
    for (const [file, text] of Object.entries(sources)) {
      for (const match of text.matchAll(CALL)) {
        if (match[4]) hidden.push(`${match[2]} (${file})`)
      }
    }
    expect(hidden).toEqual([])
  })
})
