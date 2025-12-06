import React from 'react'
import { CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui'

export interface SystemInfoItem {
  label: string
  value: string
  status?: 'healthy' | 'unhealthy' | 'unknown'
}

export interface SystemInfoCardProps {
  title?: string
  description?: string
  items: SystemInfoItem[]
}

export const SystemInfoCard: React.FC<SystemInfoCardProps> = ({
  title = 'System Information',
  description,
  items,
}) => {
  const renderStatus = (status?: string) => {
    if (!status) return null

    switch (status) {
      case 'healthy':
        return <CheckCircleIcon className="h-5 w-5 text-green-500 mr-2" />
      case 'unhealthy':
        return <XCircleIcon className="h-5 w-5 text-red-500 mr-2" />
      default:
        return <div className="h-5 w-5 rounded-full bg-gray-300 mr-2" />
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent>
        <dl className="grid grid-cols-1 gap-x-4 gap-y-6 sm:grid-cols-2">
          {items.map((item, index) => (
            <div key={index}>
              <dt className="text-sm font-medium text-gray-500">{item.label}</dt>
              <dd className="mt-1 text-sm text-gray-900 flex items-center">
                {renderStatus(item.status)}
                {item.value}
              </dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  )
}

export default SystemInfoCard
