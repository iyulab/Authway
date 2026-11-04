import React, { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { useTranslation } from 'react-i18next'
import i18n from '@/i18n'
import { apiErrorMessage, refusalAt, tenantsApi, type Tenant, type TenantSettings } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'
import { Button, Input, Select, Textarea } from '@/components/ui'

// What the API accepts (pkg/tenant TenantSettings): a password length of
// 8–72 when more than every tenant gets, a session of up to 30 days, an
// audit log kept at least 30 days (a year when not said).
const MIN_PASSWORD = 8
const MAX_PASSWORD = 72
const MAX_SESSION_MINUTES = 43200
const MIN_AUDIT_DAYS = 30
const MAX_AUDIT_DAYS = 36500

interface Draft {
  name: string
  description: string
  passwordMinLength: string
  sessionTimeout: string
  auditRetentionDays: string
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
  auditRetentionDays: 'settings.audit_retention_days',
  allowedDomains: 'settings.allowed_domains',
  signupMode: 'settings.signup_mode',
}

const draftOf = (t: Tenant): Draft => ({
  name: t.name,
  description: t.description ?? '',
  passwordMinLength: t.settings?.password_min_length ? String(t.settings.password_min_length) : '',
  sessionTimeout: t.settings?.session_timeout ? String(t.settings.session_timeout) : '',
  auditRetentionDays: t.settings?.audit_retention_days ? String(t.settings.audit_retention_days) : '',
  allowedDomains: (t.settings?.allowed_domains ?? []).join('\n'),
  signupMode: t.settings?.signup_mode === 'open' ? 'open' : 'invite_only',
})

/** The draft's problems, by field; empty when it can be sent. */
export function draftErrors(d: Draft): Partial<Record<keyof Draft, string>> {
  const errors: Partial<Record<keyof Draft, string>> = {}
  if (d.name.trim().length < 2) errors.name = i18n.t('settings:form.nameShort')
  const pw = d.passwordMinLength.trim()
  if (pw !== '' && !(/^\d+$/.test(pw) && +pw >= MIN_PASSWORD && +pw <= MAX_PASSWORD)) {
    errors.passwordMinLength = i18n.t('settings:form.passwordMinInvalid', { min: MIN_PASSWORD, max: MAX_PASSWORD })
  }
  const st = d.sessionTimeout.trim()
  if (st !== '' && !(/^\d+$/.test(st) && +st <= MAX_SESSION_MINUTES)) {
    errors.sessionTimeout = i18n.t('settings:form.sessionInvalid', { max: MAX_SESSION_MINUTES })
  }
  const ar = d.auditRetentionDays.trim()
  if (ar !== '' && !(/^\d+$/.test(ar) && +ar >= MIN_AUDIT_DAYS && +ar <= MAX_AUDIT_DAYS)) {
    errors.auditRetentionDays = i18n.t('settings:form.auditInvalid', { min: MIN_AUDIT_DAYS, max: MAX_AUDIT_DAYS })
  }
  return errors
}

/** The settings object the draft stands for — sent whole, as the API replaces it whole. */
export function settingsOf(d: Draft, current?: TenantSettings): TenantSettings {
  return {
    ...current,
    password_min_length: d.passwordMinLength.trim() === '' ? 0 : Number(d.passwordMinLength),
    session_timeout: d.sessionTimeout.trim() === '' ? 0 : Number(d.sessionTimeout),
    audit_retention_days: d.auditRetentionDays.trim() === '' ? 0 : Number(d.auditRetentionDays),
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
  const { t } = useTranslation('settings')
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
      toast.success(t('form.saved'))
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'settings:form.saveFailed'))
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
        <Input label={t('form.name')} value={draft.name} onChange={set('name')} error={errorAt('name')} />
        <Select
          label={t('form.signupMode')}
          value={draft.signupMode}
          onChange={set('signupMode')}
          error={errorAt('signupMode')}
          options={[
            { value: 'invite_only', label: t('form.inviteOnly') },
            { value: 'open', label: t('form.open') },
          ]}
        />
        <Input
          label={t('form.passwordMin')}
          inputMode="numeric"
          placeholder={String(MIN_PASSWORD)}
          value={draft.passwordMinLength}
          onChange={set('passwordMinLength')}
          error={errorAt('passwordMinLength')}
          helperText={t('form.passwordMinHelp')}
        />
        <Input
          label={t('form.session')}
          inputMode="numeric"
          placeholder={t('form.sessionPlaceholder')}
          value={draft.sessionTimeout}
          onChange={set('sessionTimeout')}
          error={errorAt('sessionTimeout')}
          helperText={t('form.sessionHelp')}
        />
        <Input
          label={t('form.audit')}
          inputMode="numeric"
          placeholder="365"
          value={draft.auditRetentionDays}
          onChange={set('auditRetentionDays')}
          error={errorAt('auditRetentionDays')}
          helperText={t('form.auditHelp')}
        />
      </div>
      <Textarea
        label={t('form.domains')}
        rows={2}
        placeholder={t('form.domainsPlaceholder')}
        value={draft.allowedDomains}
        onChange={set('allowedDomains')}
        error={errorAt('allowedDomains')}
        helperText={t('form.domainsHelp')}
      />
      <Textarea label={t('form.description')} rows={2} value={draft.description} onChange={set('description')} error={errorAt('description')} />
      <div className="flex justify-end">
        <Button type="submit" disabled={Object.keys(errors).length > 0} isLoading={save.isPending}>
          {t('form.save')}
        </Button>
      </div>
    </form>
  )
}

export default TenantSettingsForm
