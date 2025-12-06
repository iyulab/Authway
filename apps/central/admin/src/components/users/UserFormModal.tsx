import React from 'react'
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
  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Edit User" size="md">
      <UserForm
        initialData={user}
        onSubmit={onSubmit}
        onCancel={onClose}
        isSubmitting={isSubmitting}
        submitLabel="Update"
      />
    </Modal>
  )
}

export default UserFormModal
