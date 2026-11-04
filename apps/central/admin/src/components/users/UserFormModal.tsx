import React from 'react'
import { useTranslation } from 'react-i18next'
import { Modal } from '@/components/ui'
import { UserForm, UserFormData } from './UserForm'
import { User } from '@/lib/api'

export interface UserFormModalProps {
  isOpen: boolean
  onClose: () => void
  onSubmit: (data: UserFormData) => void
  user: User
  isSubmitting?: boolean
}

export const UserFormModal: React.FC<UserFormModalProps> = ({
  isOpen,
  onClose,
  onSubmit,
  user,
  isSubmitting = false,
}) => {
  const { t } = useTranslation('users')
  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t('form.modalTitle')} size="md">
      <UserForm
        initialData={user}
        onSubmit={onSubmit}
        onCancel={onClose}
        isSubmitting={isSubmitting}
        submitLabel={t('form.update')}
      />
    </Modal>
  )
}

export default UserFormModal
