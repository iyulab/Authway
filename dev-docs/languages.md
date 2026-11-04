# Languages

**Question:** in which language does a person read Authway, and who decides?
**Level:** L3 · **Source:** this page; behavior is pinned by the locale tests of
each part (`apps/branding/auth-ui/src/i18n`, `apps/central/admin/src/i18n`,
`packages/mail`)

Authway speaks two languages everywhere a person reads it: Korean (`ko`) and
English (`en`). English is the fallback. A person never meets a screen, an error
or a mail that is half in one language and half in the other.

## Who decides

Every part that shows text to a person picks the language the same way, in this
order, and stops at the first that names a supported language:

1. the `lang` query parameter of the address (`?lang=ko`);
2. the choice the person made with the language switcher, kept in the browser;
3. the languages the browser prefers (a regional tag such as `ko-KR` reads as
   its language);
4. English.

An application may pass `ui_locales` on the authorization request. The sign-in
screen follows it when it names a supported language, unless the address itself
carries `lang`.

The login screens, the administration console, the React SDK components, the
landing page and the samples follow this rule. Each one shows a language
switcher on every screen it has, and says which language the document is in
(`<html lang>`).

## Who words what

| Text | Worded by | Why |
|---|---|---|
| Screens, labels, validation messages | The part that shows them, from locale files that carry the same keys in both languages | A missing or untranslated key fails the tests: they compare the files with each other and with every key a screen asks for |
| A refusal from the API | The screen, from the refusal's `code` | The API words a refusal in one language for a developer; the `code` is the stable identifier a screen branches on. The login screens never show the API's `detail`: a code without a translation shows the screen's own translated sentence. The console does the same, except that a refusal naming a field of a form keeps the API's text so the field error stays specific |
| Mail | The API, in the language the request asked for | A mail is sent while the person's browser is the only thing that knows their language |

## Mail

`packages/mail` holds each message once per language (`locales/ko.json`,
`locales/en.json`) over shared templates. A request that sends mail carries
`Accept-Language`; the login screens and the administration console send the
language the person chose there, not the one the browser prefers. The API picks
the best supported language of that header, and English when there is none.

An invitation is the one mail whose reader is not the one who asks for it, and
who is not yet a user: Authway does not know their language, and the server that
sends the invitation on an application's behalf sends no `Accept-Language`. So
the call that creates or resends an invitation can name the language
(`locale`, one tag), and that wins over the header. A language there is no mail
in is refused (`400 invalid_request`, `field: locale`), never answered in
another one. Every other mail is requested by the person who reads it, in their
own language. Authway stores no language for a tenant, a user or an invitation:
a resend that names none follows its own `Accept-Language`.

Every implementation of Authway renders these same files, so a language is added
once: a locale file with the same keys and the same `{{values}}`, and the
language joins the list the negotiation chooses from.
