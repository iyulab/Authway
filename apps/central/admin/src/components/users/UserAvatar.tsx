import React from 'react'
import { cn } from '@/lib/utils'

export interface UserAvatarProps {
  name?: string
  email: string
  avatarUrl?: string
  size?: 'sm' | 'md' | 'lg'
  className?: string
}

const sizeStyles = {
  sm: 'h-8 w-8 text-xs',
  md: 'h-10 w-10 text-sm',
  lg: 'h-12 w-12 text-base',
}

export const UserAvatar: React.FC<UserAvatarProps> = ({
  name,
  email,
  avatarUrl,
  size = 'md',
  className,
}) => {
  // Get initials from name or fall back to email first character
  const getInitials = () => {
    if (name) {
      const parts = name.trim().split(' ')
      if (parts.length >= 2) {
        return `${parts[0][0]}${parts[parts.length - 1][0]}`.toUpperCase()
      }
      return name[0].toUpperCase()
    }
    return email[0].toUpperCase()
  }

  if (avatarUrl) {
    return (
      <img
        src={avatarUrl}
        alt={name || email}
        className={cn('rounded-full object-cover', sizeStyles[size], className)}
      />
    )
  }

  return (
    <div
      className={cn(
        'rounded-full bg-gray-300 flex items-center justify-center font-medium text-gray-700',
        sizeStyles[size],
        className
      )}
    >
      {getInitials()}
    </div>
  )
}

export default UserAvatar
