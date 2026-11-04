import { describe, it, expect } from 'vitest'

// Every translation key the source names exists in English (and so, by
// locales.test.ts, in Korean). Keys built at run time (`t(`nav.${x}`)`) are
// checked by the prefixes they start with.
const sources = import.meta.glob<string>('../**/*.{ts,tsx}', { eager: true, query: '?raw', import: 'default' })
const locales = import.meta.glob<Record<string, unknown>>('./locales/en/*.json', { eager: true, import: 'default' })

const english: Record<string, Record<string, unknown>> = {}
for (const [path, tree] of Object.entries(locales)) {
  english[/\/([^/]+)\.json$/.exec(path)![1]] = tree
}

const lookup = (namespace: string, key: string): unknown =>
  key.split('.').reduce<unknown>((node, part) => (node && typeof node === 'object' ? (node as Record<string, unknown>)[part] : undefined), english[namespace])

const defaultNamespace = (source: string): string | undefined => {
  const hook = /useTranslation\(\s*(?:\[\s*)?'(\w+)'/.exec(source)
  return hook?.[1]
}

describe('translation keys in the source', () => {
  const used: { file: string; namespace: string; key: string; dynamic: boolean }[] = []

  for (const [path, source] of Object.entries(sources)) {
    if (/\.test\.tsx?$/.test(path) || path.includes('/test/') || path.includes('/i18n/')) continue
    const fallback = defaultNamespace(source)
    // t('key'), t(`key.${x}`), i18n.t('ns:key'), i18nKey="key", and the
    // 'ns:key' arguments of apiErrorMessage.
    const patterns = [
      /\bt\(\s*(['`])((?:(?!\1).)+)\1/g,
      /\bi18nKey=(["'])((?:(?!\1).)+)\1/g,
      /apiErrorMessage\([^)]*?(['"])(\w+:[\w.]+)\1/g,
      /(?:fallbackKey|overrides)\b[^'"]*(['"])(\w+:[\w.]+)\1/g,
    ]
    for (const pattern of patterns) {
      for (const match of source.matchAll(pattern)) {
        let key = match[2]
        const dynamic = key.includes('${')
        if (dynamic) key = key.slice(0, key.indexOf('${')).replace(/\.$/, '')
        let namespace = fallback
        if (/^\w+:/.test(key)) [namespace, key] = key.split(':') as [string, string]
        if (!namespace || !key) continue
        used.push({ file: path, namespace, key, dynamic })
      }
    }
  }

  it('finds the keys the pages use', () => {
    expect(used.length).toBeGreaterThan(300)
  })

  it('all exist', () => {
    const missing = used
      .filter(({ namespace, key }) => lookup(namespace, key) === undefined)
      .map(({ file, namespace, key }) => `${file}: ${namespace}:${key}`)
    expect(missing).toEqual([])
  })
})
