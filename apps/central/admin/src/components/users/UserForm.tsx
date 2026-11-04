import React from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import i18n from '@/i18n'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Input, Button } from '@/components/ui'
import { User } from '@/lib/api'

// User form validation schema - only name and avatar_url are editable
export const userFormSchema = z.object({
  name: z.string().min(1, { error: () => i18n.t('users:form.nameRequired') }),
  avatar_url: z.string().url({ error: () => i18n.t('users:form.invalidUrl') }).optional().or(z.literal('')),
})

export type UserFormData = z.infer<typeof userFormSchema>

export interface UserFormProps {
  initialData?: User
  onSubmit: (data: UserFormData) => void
  onCancel: () => void
  isSubmitting?: boolean
  submitLabel?: string
}

export const UserForm: React.FC<UserFormProps> = ({
  initialData,
  onSubmit,
  onCancel,
  isSubmitting = false,
  submitLabel,
}) => {
  const { t } = useTranslation('users')
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<UserFormData>({
    resolver: zodResolver(userFormSchema),
    defaultValues: initialData
      ? {
          name: initialData.name || '',
          avatar_url: initialData.avatar_url || '',
        }
      : {
          name: '',
          avatar_url: '',
        },
  })

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
      {/* Read-only fields */}
      {initialData && (
        <div className="space-y-3 mb-4 p-3 bg-gray-50 rounded-lg">
          <div>
            <label className="block text-sm font-medium text-gray-500">{t('form.email')}</label>
            <p className="mt-1 text-sm text-gray-900">{initialData.email}</p>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-500">{t('form.connected')}</label>
              <p className="mt-1 text-sm text-gray-900 capitalize">
                {initialData.connections?.length ? initialData.connections.join(', ') : t('form.none')}
              </p>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-500">{t('form.status')}</label>
              <p className="mt-1 text-sm text-gray-900">
                {initialData.active ? t('form.active') : t('form.inactive')} / {initialData.email_verified ? t('form.emailVerified') : t('form.emailNotVerified')}
              </p>
            </div>
          </div>
        </div>
      )}

      <Input
        {...register('name')}
        label={t('form.name')}
        placeholder={t('form.namePlaceholder')}
        error={errors.name?.message}
      />

      <Input
        {...register('avatar_url')}
        label={t('form.avatar')}
        placeholder="https://example.com/avatar.jpg"
        error={errors.avatar_url?.message}
        helperText={t('form.avatarHelp')}
      />

      <div className="flex justify-end space-x-3 pt-4">
        <Button type="button" variant="secondary" onClick={onCancel}>
          {t('form.cancel')}
        </Button>
        <Button type="submit" isLoading={isSubmitting}>
          {submitLabel ?? t('form.update')}
        </Button>
      </div>
    </form>
  )
}

export default UserForm
