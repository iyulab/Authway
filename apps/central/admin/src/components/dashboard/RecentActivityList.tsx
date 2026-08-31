import React from 'react'
import { CheckCircleIcon } from '@heroicons/react/24/outline'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui'

export interface ActivityItem {
  id: string
  name: string
  subtitle: string
  icon: React.ElementType
  iconColor?: string
  active?: boolean
}

export interface RecentActivityListProps {
  title: string
  description?: string
  items: ActivityItem[]
  emptyMessage?: string
  maxItems?: number
}

export const RecentActivityList: React.FC<RecentActivityListProps> = ({
  title,
  description,
  items,
  emptyMessage = 'No items to display',
  maxItems = 5,
}) => {
  const displayItems = items.slice(0, maxItems)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent noPadding>
        <div className="divide-y divide-gray-200">
          {displayItems.length > 0 ? (
            displayItems.map((item) => {
              const Icon = item.icon
              return (
                <div key={item.id} className="px-4 py-4 sm:px-6">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center">
                      <div className="shrink-0">
                        <Icon
                          className={`h-8 w-8 ${item.iconColor || 'text-gray-400'}`}
                        />
                      </div>
                      <div className="ml-4">
                        <div className="text-sm font-medium text-gray-900">
                          {item.name}
                        </div>
                        <div className="text-sm text-gray-500">{item.subtitle}</div>
                      </div>
                    </div>
                    <div className="flex items-center">
                      {item.active !== undefined && (
                        item.active ? (
                          <CheckCircleIcon className="h-5 w-5 text-green-500" />
                        ) : (
                          <div className="h-5 w-5 rounded-full bg-gray-300" />
                        )
                      )}
                    </div>
                  </div>
                </div>
              )
            })
          ) : (
            <div className="px-4 py-8 text-center text-gray-500">
              {emptyMessage}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

export default RecentActivityList
