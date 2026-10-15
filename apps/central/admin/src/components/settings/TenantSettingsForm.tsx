import React, { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { refusalAt, tenantsApi, type Tenant, type TenantSettings } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'
import { Button, Input, Select, Textarea } from '@/components/ui'

// What the API accepts (pkg/tenant TenantSettings): a password length of
// 8–72 when more than every tenant gets, a session of up to 30 days.
const MIN_PASSWORD = 8
const MAX_PASSWORD = 72
const MAX_SESSION_MINUTES = 43200

interface Draft {
  name: string
  description: string
  passwordMinLength: string
  sessionTimeout: string
  allowedDomains: string
  signupMode: string
}

// The request member each draft field is sent as: a refusal from the API
// names that member, and the form shows it at the field.
const memberOf: Record<keyof Draft, string> = {
  name: 'name',
  description: 'description',
  passwordMinLength: 'settings.password_min_length',
  sessionTimeout: 'settings.session_timeout',
  allowedDomains: 'settings.allowed_domains',
  signupMode: 'settings.signup_mode',
}

const draftOf = (t: Tenant): Draft => ({
  name: t.name,
  description: t.description ?? '',
  passwordMinLength: t.settings?.password_min_length ? String(t.settings.password_min_length) : '',
  sessionTimeout: t.settings?.session_timeout ? String(t.settings.session_timeout) : '',
  allowedDomains: (t.settings?.allowed_domains ?? []).join('\n'),
  signupMode: t.settings?.signup_mode === 'open' ? 'open' : 'invite_only',
})

/** The draft's problems, by field; empty when it can be sent. */
export function draftErrors(d: Draft): Partial<Record<keyof Draft, string>> {
  const errors: Partial<Record<keyof Draft, string>> = {}
  if (d.name.trim().length < 2) errors.name = '이름은 두 글자 이상입니다'
  const pw = d.passwordMinLength.trim()
  if (pw !== '' && !(/^\d+$/.test(pw) && +pw >= MIN_PASSWORD && +pw <= MAX_PASSWORD)) {
    errors.passwordMinLength = `${MIN_PASSWORD}~${MAX_PASSWORD} 사이의 수, 또는 비워 두세요(${MIN_PASSWORD})`
  }
  const st = d.sessionTimeout.trim()
  if (st !== '' && !(/^\d+$/.test(st) && +st <= MAX_SESSION_MINUTES)) {
    errors.sessionTimeout = `0~${MAX_SESSION_MINUTES} 분(30 일), 또는 비워 두세요(기본)`
  }
  return errors
}

/** The settings object the draft stands for — sent whole, as the API replaces it whole. */
export function settingsOf(d: Draft, current?: TenantSettings): TenantSettings {
  return {
    ...current,
    password_min_length: d.passwordMinLength.trim() === '' ? 0 : Number(d.passwordMinLength),
    session_timeout: d.sessionTimeout.trim() === '' ? 0 : Number(d.sessionTimeout),
    allowed_domains: d.allowedDomains
      .split(/[\s,]+/)
      .map((s) => s.trim().toLowerCase())
      .filter(Boolean),
    signup_mode: d.signupMode,
  }
}

/**
 * The selected tenant's name, description and settings. They could be set
 * only when the tenant was created; nothing in the console changed them.
 */
const TenantSettingsForm: React.FC = () => {
  const queryClient = useQueryClient()
  const { selectedTenant, setSelectedTenant } = useTenantStore()
  const id = selectedTenant?.id

  const { data: tenant } = useQuery({
    queryKey: ['tenant', id],
    queryFn: () => tenantsApi.get(id!).then((res) => res.data),
    enabled: !!id,
  })

  const [draft, setDraft] = useState<Draft | null>(null)
  useEffect(() => {
    if (tenant) setDraft(draftOf(tenant))
  }, [tenant])

  const save = useMutation({
    mutationFn: (d: Draft) =>
      tenantsApi.update(id!, {
        name: d.name.trim(),
        description: d.description.trim(),
        settings: settingsOf(d, tenant?.settings),
      }),
    onSuccess: (res) => {
      queryClient.setQueryData(['tenant', id], res.data)
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      setSelectedTenant(res.data)
      toast.success('테넌트 설정을 저장했습니다')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.detail || '테넌트 설정을 저장하지 못했습니다')
    },
  })

  if (!draft) return null
  const errors = draftErrors(draft)
  // What is wrong with a field: the form's own check first, then what the API
  // refused of the last save. Any edit makes that refusal stale.
  const errorAt = (field: keyof Draft) => errors[field] ?? refusalAt(save.error, memberOf[field])
  const set = (field: keyof Draft) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => {
    if (save.error) save.reset()
    setDraft({ ...draft, [field]: e.target.value })
  }

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (Object.keys(errors).length === 0) save.mutate(draft)
      }}
    >
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Input label="이름" value={draft.name} onChange={set('name')} error={errorAt('name')} />
        <Select
          label="가입 방식"
          value={draft.signupMode}
          onChange={set('signupMode')}
          error={errorAt('signupMode')}
          options={[
            { value: 'invite_only', label: '초대 전용 — 첫 로그인에 초대가 있어야 함' },
            { value: 'open', label: '공개 — 누구나 첫 로그인으로 계정이 생김' },
          ]}
        />
        <Input
          label="비밀번호 최소 길이"
          inputMode="numeric"
          placeholder={String(MIN_PASSWORD)}
          value={draft.passwordMinLength}
          onChange={set('passwordMinLength')}
          error={errorAt('passwordMinLength')}
          helperText="모든 테넌트의 바닥은 8 자입니다. 올리기만 할 수 있습니다."
        />
        <Input
          label="로그인 세션(분)"
          inputMode="numeric"
          placeholder="기본"
          value={draft.sessionTimeout}
          onChange={set('sessionTimeout')}
          error={errorAt('sessionTimeout')}
          helperText="사용자의 로그인 세션이 유지되는 시간. 최대 43200 분(30 일)."
        />
      </div>
      <Textarea
        label="허용 이메일 도메인"
        rows={2}
        placeholder="example.com"
        value={draft.allowedDomains}
        onChange={set('allowedDomains')}
        error={errorAt('allowedDomains')}
        helperText="이름이 있으면 이 도메인의 주소로만 계정이 생깁니다(초대·첫 로그인). 비우면 제한 없음. 이미 있는 계정은 그대로."
      />
      <Textarea label="설명" rows={2} value={draft.description} onChange={set('description')} error={errorAt('description')} />
      <div className="flex justify-end">
        <Button type="submit" disabled={Object.keys(errors).length > 0} isLoading={save.isPending}>
          저장
        </Button>
      </div>
    </form>
  )
}

export default TenantSettingsForm
