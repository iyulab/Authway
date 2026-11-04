import { useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import i18n from '@/i18n'
import { apiErrorMessage, tenantsApi, type Tenant } from './api'

/**
 * Turns a tenant off or on — only `active` is sent. Off, none of its clients
 * signs anyone in or gets tokens; on, each is as its own setting says. It is
 * a platform action: an operator's console account needs no support access.
 */
export function useTenantActive(onDone?: (tenant: Tenant | null) => void) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ tenant, active }: { tenant: Tenant; active: boolean }) => tenantsApi.update(tenant.id, { active }),
    onSuccess: (res, { active }) => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      toast.success(i18n.t(active ? 'tenants:active.turnedOn' : 'tenants:active.turnedOff'))
      onDone?.(res.data)
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'tenants:active.failed', 'tenants:active.refusals'))
      onDone?.(null)
    },
  })
}
