import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { SupportAccess, Tenant } from '@/lib/api'

interface TenantState {
  selectedTenant: Tenant | null
  /** The support access the operator entered the selected tenant with, if it did. */
  supportAccess: SupportAccess | null
  setSelectedTenant: (tenant: Tenant | null, supportAccess?: SupportAccess | null) => void
  clearSelectedTenant: () => void
}

export const useTenantStore = create<TenantState>()(
  persist(
    (set) => ({
      selectedTenant: null,
      supportAccess: null,
      setSelectedTenant: (tenant, supportAccess = null) => set({ selectedTenant: tenant, supportAccess }),
      clearSelectedTenant: () => set({ selectedTenant: null, supportAccess: null }),
    }),
    {
      name: 'authway-admin-tenant',
      partialize: (state) => ({
        selectedTenant: state.selectedTenant,
        supportAccess: state.supportAccess,
      }),
    }
  )
)
