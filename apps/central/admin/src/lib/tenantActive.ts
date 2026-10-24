import { useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { tenantsApi, type Tenant } from './api'

const ACTIVE_REFUSALS: Record<string, string> = {
  default_tenant_protected: '기본 테넌트는 끌 수 없습니다.',
  upstream_sync_failed: '일부 클라이언트를 인가 서버에 반영하지 못했습니다. 같은 작업을 다시 하면 마저 반영됩니다.',
}

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
      toast.success(active ? '테넌트를 켰습니다' : '테넌트를 껐습니다')
      onDone?.(res.data)
    },
    onError: (error: any) => {
      const code: string | undefined = error.response?.data?.code
      toast.error((code && ACTIVE_REFUSALS[code]) || error.response?.data?.detail || '테넌트를 바꾸지 못했습니다')
      onDone?.(null)
    },
  })
}
