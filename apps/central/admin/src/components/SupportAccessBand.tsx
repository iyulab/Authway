import React, { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { ShieldExclamationIcon } from '@heroicons/react/24/outline'
import { supportAccessApi } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'

const minutesLeft = (expiresAt: string, now: number) => Math.max(0, Math.ceil((Date.parse(expiresAt) - now) / 60_000))

/**
 * While an operator is inside a tenant through support access: which tenant,
 * why, how long is left, and a way to end it. When the hour is up the console
 * goes back to the tenant list — the API would refuse the tenant's contents
 * from then on.
 */
const SupportAccessBand: React.FC = () => {
  const queryClient = useQueryClient()
  const { selectedTenant, supportAccess, clearSelectedTenant } = useTenantStore()
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!supportAccess) return
    const timer = setInterval(() => setNow(Date.now()), 15_000)
    return () => clearInterval(timer)
  }, [supportAccess])

  const expired = supportAccess ? Date.parse(supportAccess.expires_at) <= now : false
  useEffect(() => {
    if (expired) {
      clearSelectedTenant()
      toast('지원 접근 시간이 끝났습니다. 다시 들어가려면 사유를 적어 새로 여세요.')
    }
  }, [expired, clearSelectedTenant])

  const end = useMutation({
    mutationFn: (id: string) => supportAccessApi.end(id),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['support-access'] })
      clearSelectedTenant()
    },
  })

  if (!supportAccess || !selectedTenant || expired) return null
  return (
    <div role="status" className="bg-amber-50 border-b border-amber-200 px-6 py-2 flex items-center gap-3 text-sm text-amber-900">
      <ShieldExclamationIcon className="h-5 w-5 shrink-0" aria-hidden="true" />
      <span className="flex-1 min-w-0 truncate">
        <strong>{selectedTenant.name}</strong> 지원 접근 중 — {supportAccess.reason} · {minutesLeft(supportAccess.expires_at, now)}분 남음 ·
        이 테넌트의 감사 로그에 남습니다
      </span>
      <button
        type="button"
        onClick={() => end.mutate(supportAccess.id)}
        disabled={end.isPending}
        className="px-3 py-1 rounded-md bg-white border border-amber-300 hover:bg-amber-100 disabled:opacity-50"
      >
        지원 접근 끝내기
      </button>
    </div>
  )
}

export default SupportAccessBand
