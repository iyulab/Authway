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
})
