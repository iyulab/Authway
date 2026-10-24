import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen, waitFor } from '@/test/utils'
import toast from 'react-hot-toast'
import SettingsPage from './SettingsPage'
import { authApi, capabilitiesApi, healthApi, tenantsApi } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'

// The settings page shows only what it reads from the deployment, and says
// why a tenant could not be deleted.

const answer = <T,>(data: T) => ({ data }) as never

beforeEach(() => {
  vi.restoreAllMocks()
  useTenantStore.setState({ selectedTenant: { id: 't1', name: 'Tenant', slug: 'tenant', active: true } as never })
  vi.spyOn(authApi, 'validate').mockResolvedValue(answer({ valid: true, info: { authenticated: true, version: 'v20261015212924' } }))
  vi.spyOn(healthApi, 'get').mockResolvedValue(answer({ status: 'ok', version: 'v20261015212924', migrations: ['000', '040', '041'] }))
  vi.spyOn(capabilitiesApi, 'get').mockResolvedValue(
    answer({ multi_tenant: true, providers: ['google'], magic_link: true, mfa: ['totp'], signup_modes: ['invite_only', 'open'], device_authorization: true, token_exchange: false, ciba: false }),
  )
})

describe('settings', () => {
  it('shows the deployed version, the last migration and what the deployment offers', async () => {
    render(<SettingsPage />)
    expect((await screen.findAllByText('v20261015212924')).length).toBeGreaterThan(0)
    expect(screen.getByText('migration 041')).toBeInTheDocument()
    expect(await screen.findByText('google')).toBeInTheDocument()
    expect(screen.getByText('invite_only, open')).toBeInTheDocument()
    // Nothing the console cannot read.
    expect(screen.queryByText(/PKCE/)).not.toBeInTheDocument()
    expect(screen.queryByText('http://localhost:4445')).not.toBeInTheDocument()
    expect(screen.queryByText('v1.0.0-alpha')).not.toBeInTheDocument()
  })

  it('says why a tenant that still has users was not deleted', async () => {
    const user = userEvent.setup()
    const error = vi.spyOn(toast, 'error')
    vi.spyOn(tenantsApi, 'delete').mockRejectedValue({ response: { status: 409, data: { code: 'tenant_has_users', detail: 'Cannot delete tenant with existing users' } } })
    render(<SettingsPage />)

    await user.click(await screen.findByRole('button', { name: /테넌트 삭제/ }))
    await user.type(screen.getByPlaceholderText('tenant'), 'tenant')
    const confirm = screen.getAllByRole('button', { name: /테넌트 삭제/ }).at(-1)!
    await user.click(confirm)
    await waitFor(() => expect(error).toHaveBeenCalledWith('사용자가 남아 있어 지울 수 없습니다. 먼저 사용자를 지우세요.'))
  })

  it('turns the tenant off after asking, and on again', async () => {
    const user = userEvent.setup()
    const update = vi
      .spyOn(tenantsApi, 'update')
      .mockResolvedValueOnce(answer({ id: 't1', name: 'Tenant', slug: 'tenant', active: false }))
      .mockResolvedValueOnce(answer({ id: 't1', name: 'Tenant', slug: 'tenant', active: true }))
    render(<SettingsPage />)

    await user.click(await screen.findByRole('button', { name: '테넌트 끄기' }))
    expect(update).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: '끄기' }))
    await waitFor(() => expect(update).toHaveBeenCalledWith('t1', { active: false }))
    expect(useTenantStore.getState().selectedTenant?.active).toBe(false)

    await user.click(await screen.findByRole('button', { name: '테넌트 켜기' }))
    await waitFor(() => expect(update).toHaveBeenLastCalledWith('t1', { active: true }))
    expect(useTenantStore.getState().selectedTenant?.active).toBe(true)
  })

  it('says the default tenant cannot be turned off', async () => {
    const user = userEvent.setup()
    const error = vi.spyOn(toast, 'error')
    vi.spyOn(tenantsApi, 'update').mockRejectedValue({ response: { status: 403, data: { code: 'default_tenant_protected', detail: 'Cannot deactivate the default tenant' } } })
    render(<SettingsPage />)

    await user.click(await screen.findByRole('button', { name: '테넌트 끄기' }))
    await user.click(screen.getByRole('button', { name: '끄기' }))
    await waitFor(() => expect(error).toHaveBeenCalledWith('기본 테넌트는 끌 수 없습니다.'))
    expect(useTenantStore.getState().selectedTenant?.active).toBe(true)
  })

  it("changes the tenant's settings, sending them whole, and clears the description", async () => {
    const user = userEvent.setup()
    const tenant = {
      id: 't1', name: 'Tenant', slug: 'tenant', active: true, description: 'old',
      settings: { password_min_length: 10, session_timeout: 60, allowed_domains: ['a.example'], signup_mode: 'invite_only' },
    }
    vi.spyOn(tenantsApi, 'get').mockResolvedValue(answer(tenant))
    const update = vi.spyOn(tenantsApi, 'update').mockResolvedValue(answer({ ...tenant, description: '' }))
    render(<SettingsPage />)

    const length = await screen.findByLabelText('비밀번호 최소 길이')
    expect(length).toHaveValue('10')
    await user.clear(length)
    await user.type(length, '7')
    expect(screen.getByRole('button', { name: '저장' })).toBeDisabled()
    await user.clear(length)
    await user.type(length, '12')
    await user.type(screen.getByLabelText('허용 이메일 도메인'), '{Enter}B.example')
    await user.selectOptions(screen.getByLabelText('가입 방식'), 'open')
    await user.clear(screen.getByLabelText('설명'))
    await user.click(screen.getByRole('button', { name: '저장' }))

    await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    expect(update.mock.calls[0]).toEqual([
      't1',
      {
        name: 'Tenant',
        description: '',
        settings: { password_min_length: 12, session_timeout: 60, audit_retention_days: 0, allowed_domains: ['a.example', 'b.example'], signup_mode: 'open' },
      },
    ])
  })

  it('shows a refused setting at its field', async () => {
    const user = userEvent.setup()
    const tenant = { id: 't1', name: 'Tenant', slug: 'tenant', active: true, settings: { signup_mode: 'invite_only' } }
    vi.spyOn(tenantsApi, 'get').mockResolvedValue(answer(tenant))
    vi.spyOn(tenantsApi, 'update').mockRejectedValue({
      response: { status: 400, data: { code: 'invalid_request', field: 'settings.allowed_domains', detail: 'Invalid value for settings.allowed_domains' } },
    })
    render(<SettingsPage />)

    await user.type(await screen.findByLabelText('허용 이메일 도메인'), 'not a domain')
    await user.click(screen.getByRole('button', { name: '저장' }))
    expect(await screen.findByText('Invalid value for settings.allowed_domains')).toBeInTheDocument()
  })
})
