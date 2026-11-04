import { describe, it, expect } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import i18n, { currentLanguage, intlLocale } from './index'
import { apiErrorMessage, refusalMessage } from './problem'
import { formatDay, formatNumber } from '@/lib/utils'
import LanguageSwitcher from '@/components/LanguageSwitcher'
import LoginPage from '@/pages/LoginPage'
import { inLanguage } from '@/test/language'

const refusal = (data: Record<string, unknown>) => ({ response: { status: 400, data } })

describe('the language switcher', () => {
  it('offers both languages and switches the console, the page language and the remembered choice', async () => {
    const user = userEvent.setup()
    render(<LanguageSwitcher />)

    const group = screen.getByRole('group', { name: 'Language' })
    expect(group).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'English' })).toHaveAttribute('aria-pressed', 'true')

    await user.click(screen.getByRole('button', { name: '한국어' }))

    expect(currentLanguage()).toBe('ko')
    expect(document.documentElement.lang).toBe('ko')
    expect(document.title).toBe('Authway 관리 콘솔')
    expect(screen.getByRole('group', { name: '언어' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '한국어' })).toHaveAttribute('aria-pressed', 'true')
    expect(window.localStorage.setItem).toHaveBeenCalledWith('authway-language', 'ko')

    await user.click(screen.getByRole('button', { name: 'English' }))
    expect(document.documentElement.lang).toBe('en')
    expect(document.title).toBe('Authway Admin Dashboard')
  })

  it('is on the sign-in screen, before anyone is signed in', () => {
    render(
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>,
    )
    expect(screen.getByRole('group', { name: 'Language' })).toBeInTheDocument()
    expect(screen.getByText('Authway administrator sign-in')).toBeInTheDocument()
  })
})

describe('a regional browser language', () => {
  it('is the one language the console has for it', async () => {
    await i18n.changeLanguage('ko-KR')
    expect(currentLanguage()).toBe('ko')
    expect(intlLocale()).toBe('ko-KR')
    await i18n.changeLanguage('en-GB')
    expect(currentLanguage()).toBe('en')
    expect(intlLocale()).toBe('en-US')
  })

  it('falls back to English for a language the console lacks', async () => {
    await i18n.changeLanguage('fr')
    expect(currentLanguage()).toBe('en')
    expect(i18n.t('common:actions.cancel')).toBe('Cancel')
  })
})

describe('words for a failed request', () => {
  describe('in English', () => {
    it('are the code’s translation before anything else', () => {
      const error = refusal({ code: 'tenant_slug_taken', detail: 'A tenant with this slug already exists' })
      expect(apiErrorMessage(error, 'tenants:create.failed')).toBe('A tenant with this slug already exists.')
    })

    it('are the page’s own fallback when the code has no translation', () => {
      const error = refusal({ code: 'a_code_from_the_future', detail: 'the server’s words' })
      expect(apiErrorMessage(error, 'tenants:create.failed')).toBe('Failed to create tenant')
    })

    it('are the API’s detail only when there is neither', () => {
      expect(apiErrorMessage(refusal({ code: 'a_code_from_the_future', detail: 'the server’s words' }))).toBe('the server’s words')
      expect(apiErrorMessage(new Error('offline'))).toBe('Something went wrong.')
    })

    it('prefer a page’s wording of a code over the general one', () => {
      const error = refusal({ code: 'default_tenant_protected' })
      expect(apiErrorMessage(error, 'tenants:active.failed', 'tenants:active.refusals')).toBe('The default tenant cannot be turned off.')
      expect(apiErrorMessage(error, 'settings:toast.deleteFailed', 'settings:deleteRefusals')).toBe('The default tenant cannot be deleted.')
    })

    it('keep the API’s own words for a general code that names a field', () => {
      const error = refusal({ code: 'invalid_request', field: 'url', detail: 'url must not point at a local or private address' })
      expect(refusalMessage(error.response.data)).toBe('url must not point at a local or private address')
      expect(refusalMessage({ code: 'invalid_request' })).toBe('The request is not valid.')
    })
  })

  describe('in Korean', () => {
    inLanguage('ko')

    it('are Korean', () => {
      const error = refusal({ code: 'tenant_slug_taken', detail: 'A tenant with this slug already exists' })
      expect(apiErrorMessage(error, 'tenants:create.failed')).toBe('같은 슬러그의 테넌트가 이미 있습니다.')
      expect(apiErrorMessage(refusal({ code: 'a_code_from_the_future' }), 'tenants:create.failed')).toBe('테넌트를 만들지 못했습니다')
      expect(apiErrorMessage(new Error('offline'))).toBe('오류가 발생했습니다.')
    })
  })
})

describe('dates and numbers', () => {
  it('follow the console’s language', async () => {
    const day = '2026-11-04T09:30:00Z'
    expect(formatDay(day)).toBe('11/4/2026')
    expect(formatNumber(1234567)).toBe('1,234,567')
    await i18n.changeLanguage('ko')
    expect(formatDay(day)).toBe('2026. 11. 4.')
    expect(formatNumber(1234567)).toBe('1,234,567')
  })

  it('show a dash for what is no date', () => {
    expect(formatDay('not a date')).toBe('-')
  })
})
