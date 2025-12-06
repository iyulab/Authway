import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { Tenant } from '@/lib/api'

interface TenantState {
  selectedTenant: Tenant | null
  setSelectedTenant: (tenant: Tenant | null) => void
  clearSelectedTenant: () => void
}

export const useTenantStore = create<TenantState>()(
  persist(
    (set) => ({
      selectedTenant: null,
      setSelectedTenant: (tenant) => set({ selectedTenant: tenant }),
      clearSelectedTenant: () => set({ selectedTenant: null }),
    }),
    {
      name: 'authway-admin-tenant',
      partialize: (state) => ({
        selectedTenant: state.selectedTenant,
      }),
    }
  )
)
