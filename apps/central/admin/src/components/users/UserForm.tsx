import React from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Input, Button } from '@/components/ui'
import { User } from '@/lib/api'

// User form validation schema - only name and avatar_url are editable
export const userFormSchema = z.object({
  name: z.string().min(1, 'Name is required'),
  avatar_url: z.string().url('Please enter a valid URL').optional().or(z.literal('')),
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
  submitLabel = 'Update',
}) => {
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
            <label className="block text-sm font-medium text-gray-500">Email</label>
            <p className="mt-1 text-sm text-gray-900">{initialData.email}</p>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-500">Provider</label>
              <p className="mt-1 text-sm text-gray-900 capitalize">{initialData.provider}</p>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-500">Status</label>
              <p className="mt-1 text-sm text-gray-900">
                {initialData.active ? 'Active' : 'Inactive'} / {initialData.email_verified ? 'Verified' : 'Not Verified'}
              </p>
            </div>
          </div>
        </div>
      )}

      <Input
        {...register('name')}
        label="Name *"
        placeholder="John Doe"
        error={errors.name?.message}
      />

      <Input
        {...register('avatar_url')}
        label="Avatar URL"
        placeholder="https://example.com/avatar.jpg"
        error={errors.avatar_url?.message}
        helperText="Optional URL for user's profile picture"
      />

      <div className="flex justify-end space-x-3 pt-4">
        <Button type="button" variant="secondary" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" isLoading={isSubmitting}>
          {submitLabel}
        </Button>
      </div>
    </form>
  )
}

export default UserForm
