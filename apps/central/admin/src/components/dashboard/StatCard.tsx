import React from 'react'
import { cn } from '@/lib/utils'

export interface StatCardProps {
  name: string
  value: number | string
  icon: React.ElementType
  variant?: 'blue' | 'green' | 'purple' | 'yellow' | 'red'
  change?: {
    value: number
    type: 'increase' | 'decrease'
  }
}

const variantStyles = {
  blue: {
    bgColor: 'bg-blue-50',
    textColor: 'text-blue-600',
    iconColor: 'text-blue-500',
  },
  green: {
    bgColor: 'bg-green-50',
    textColor: 'text-green-600',
    iconColor: 'text-green-500',
  },
  purple: {
    bgColor: 'bg-purple-50',
    textColor: 'text-purple-600',
    iconColor: 'text-purple-500',
  },
  yellow: {
    bgColor: 'bg-yellow-50',
    textColor: 'text-yellow-600',
    iconColor: 'text-yellow-500',
  },
  red: {
    bgColor: 'bg-red-50',
    textColor: 'text-red-600',
    iconColor: 'text-red-500',
  },
}

export const StatCard: React.FC<StatCardProps> = ({
  name,
  value,
  icon: Icon,
  variant = 'blue',
  change,
}) => {
  const styles = variantStyles[variant]
  const displayValue = typeof value === 'number' ? value.toLocaleString() : value

  return (
    <div
      className={cn(
        'relative overflow-hidden rounded-lg px-4 py-5 shadow-sm sm:px-6',
        styles.bgColor
      )}
    >
      <div className="flex items-center">
        <div className="shrink-0">
          <Icon className={cn('h-8 w-8', styles.iconColor)} />
        </div>
        <div className="ml-5 w-0 flex-1">
          <dl>
            <dt className={cn('text-sm font-medium truncate', styles.textColor)}>
              {name}
            </dt>
            <dd className="flex items-baseline">
              <div className={cn('text-2xl font-semibold', styles.textColor)}>
                {displayValue}
              </div>
              {change && (
                <span
                  className={cn(
                    'ml-2 text-sm font-medium',
                    change.type === 'increase' ? 'text-green-600' : 'text-red-600'
                  )}
                >
                  {change.type === 'increase' ? '+' : '-'}
                  {change.value}%
                </span>
              )}
            </dd>
          </dl>
        </div>
      </div>
    </div>
  )
}

export interface StatCardGridProps {
  children: React.ReactNode
  columns?: 2 | 3 | 4
}

export const StatCardGrid: React.FC<StatCardGridProps> = ({
  children,
  columns = 4,
}) => {
  const gridCols = {
    2: 'sm:grid-cols-2',
    3: 'sm:grid-cols-2 lg:grid-cols-3',
    4: 'sm:grid-cols-2 lg:grid-cols-4',
  }

  return (
    <div className={cn('grid grid-cols-1 gap-5', gridCols[columns])}>
      {children}
    </div>
  )
}

export default StatCard
