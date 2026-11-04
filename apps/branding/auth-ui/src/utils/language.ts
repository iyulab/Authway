import i18n from '../i18n'

/**
 * The language of the screen as a request header. Mail is worded by the
 * server when the request asks for it, so a request that sends mail names the
 * language the person chose here, not the one their browser happens to prefer.
 */
export function languageHeader(): Record<string, string> {
  return { 'Accept-Language': i18n.resolvedLanguage ?? i18n.language }
}
