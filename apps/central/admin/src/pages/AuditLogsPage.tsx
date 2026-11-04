import React, { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Trans, useTranslation } from 'react-i18next'
import {
  ClipboardDocumentListIcon,
  FunnelIcon,
  BuildingOfficeIcon,
  CheckCircleIcon,
  XCircleIcon,
} from '@heroicons/react/24/outline'
import { auditLogsApi, AuditLog } from '@/lib/features-api'
import {
  Button,
  Card,
  Loading,
  EmptyState,
  Badge,
  Pagination,
} from '@/components/ui'
import { useTenantStore } from '@/stores/tenant'
import { formatDateTime } from '@/lib/utils'

const AuditLogsPage: React.FC = () => {
  const { t } = useTranslation('audit')
  const { selectedTenant } = useTenantStore()
  const selectedTenantId = selectedTenant?.id || ''
  const pageSize = 20

  // State
  const [currentPage, setCurrentPage] = useState(1)
  const [filters, setFilters] = useState({
    action: '',
    severity: '',
    success: '',
  })
  const [showFilters, setShowFilters] = useState(false)

  // Fetch audit logs
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['audit-logs', selectedTenantId, currentPage, filters],
    queryFn: () =>
      auditLogsApi.list({
        tenant_id: selectedTenantId,
        action: filters.action || undefined,
        severity: filters.severity || undefined,
        success: filters.success === '' ? undefined : filters.success === 'true',
        limit: pageSize,
        offset: (currentPage - 1) * pageSize,
      }),
    enabled: !!selectedTenantId,
  })

  // Fetch summary
  const { data: summaryData } = useQuery({
    queryKey: ['audit-summary', selectedTenantId],
    queryFn: () => auditLogsApi.summary({ tenant_id: selectedTenantId }),
    enabled: !!selectedTenantId,
  })

  const { data: actionsData } = useQuery({
    queryKey: ['audit-actions'],
    queryFn: () => auditLogsApi.actions(),
    staleTime: Infinity,
  })
  const actions = actionsData?.data.actions || []

  const logs = data?.data.logs || []
  const totalLogs = data?.data.total || 0
  const totalPages = Math.ceil(totalLogs / pageSize)
  const summary = summaryData?.data.summary

  const getSeverityBadge = (severity: string) => {
    switch (severity) {
      case 'critical':
        return <Badge variant="danger">{t('severity.critical')}</Badge>
      case 'error':
        return <Badge variant="danger">{t('severity.error')}</Badge>
      case 'warning':
        return <Badge variant="warning">{t('severity.warning')}</Badge>
      default:
        return <Badge variant="info">{t('severity.info')}</Badge>
    }
  }

  // Show message if no tenant selected
  if (!selectedTenantId) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">{t('title')}</h1>
          <p className="mt-2 text-sm text-gray-700">{t('subtitle')}</p>
        </div>
        <Card className="p-8">
          <EmptyState
            icon={<BuildingOfficeIcon className="h-12 w-12" />}
            title={t('noTenant')}
            description={t('noTenantHelp')}
          />
        </Card>
      </div>
    )
  }

  if (isLoading) {
    return <Loading message={t('loading')} />
  }

  if (error) {
    return (
      <div className="text-center py-12">
        <p className="text-red-600">{t('loadFailed')}</p>
        <Button className="mt-4" onClick={() => refetch()}>{t('retry')}</Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="sm:flex sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">{t('title')}</h1>
          <p className="mt-2 text-sm text-gray-700">
            <Trans
              t={t}
              i18nKey="activity"
              values={{ name: selectedTenant?.name }}
              components={{ tenant: <span className="font-medium" /> }}
            />
          </p>
        </div>
        <Button variant="secondary" onClick={() => setShowFilters(!showFilters)}>
          <FunnelIcon className="h-5 w-5 mr-2" />
          {t('filters')}
        </Button>
      </div>

      {/* Summary Cards */}
      {summary && (
        <div className="grid grid-cols-1 md:grid-cols-5 gap-4">
          <Card className="p-4">
            <div className="text-sm text-gray-500">{t('summary.last24h')}</div>
            <div className="text-2xl font-bold text-gray-900">{summary.total_24h}</div>
          </Card>
          <Card className="p-4">
            <div className="text-sm text-gray-500">{t('summary.last7d')}</div>
            <div className="text-2xl font-bold text-gray-900">{summary.total_7d}</div>
          </Card>
          <Card className="p-4">
            <div className="text-sm text-gray-500">{t('summary.last30d')}</div>
            <div className="text-2xl font-bold text-gray-900">{summary.total_30d}</div>
          </Card>
          <Card className="p-4">
            <div className="text-sm text-gray-500">{t('summary.security')}</div>
            <div className="text-2xl font-bold text-yellow-600">{summary.security_events}</div>
          </Card>
          <Card className="p-4">
            <div className="text-sm text-gray-500">{t('summary.failed')}</div>
            <div className="text-2xl font-bold text-red-600">{summary.failed_operations}</div>
          </Card>
        </div>
      )}

      {/* Filters */}
      {showFilters && (
        <Card className="p-4">
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700">{t('filter.action')}</label>
              {/* The API refuses an action it does not know, so offer only the ones it lists. */}
              <select
                value={filters.action}
                onChange={(e) => {
                  setFilters({ ...filters, action: e.target.value })
                  setCurrentPage(1)
                }}
                className="mt-1 block w-full rounded-md border-gray-300 shadow-xs focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm"
              >
                <option value="">{t('filter.all')}</option>
                {actions.map((a) => (
                  <option key={a.action} value={a.action}>
                    {a.action}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">{t('filter.severity')}</label>
              <select
                value={filters.severity}
                onChange={(e) => setFilters({ ...filters, severity: e.target.value })}
                className="mt-1 block w-full rounded-md border-gray-300 shadow-xs focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm"
              >
                <option value="">{t('filter.all')}</option>
                <option value="info">{t('severity.info')}</option>
                <option value="warning">{t('severity.warning')}</option>
                <option value="error">{t('severity.error')}</option>
                <option value="critical">{t('severity.critical')}</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">{t('filter.status')}</label>
              <select
                value={filters.success}
                onChange={(e) => setFilters({ ...filters, success: e.target.value })}
                className="mt-1 block w-full rounded-md border-gray-300 shadow-xs focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm"
              >
                <option value="">{t('filter.all')}</option>
                <option value="true">{t('filter.success')}</option>
                <option value="false">{t('filter.failed')}</option>
              </select>
            </div>
            <div className="flex items-end">
              <Button
                variant="secondary"
                onClick={() => {
                  setFilters({ action: '', severity: '', success: '' })
                  setCurrentPage(1)
                }}
              >
                {t('filter.clear')}
              </Button>
            </div>
          </div>
        </Card>
      )}

      {/* Logs Table */}
      <Card>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.time')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.action')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.actor')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.resource')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.severity')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.status')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.ip')}</th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {logs.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-6 py-12">
                    <EmptyState
                      icon={<ClipboardDocumentListIcon className="h-12 w-12" />}
                      title={t('empty.title')}
                      description={t('empty.description')}
                    />
                  </td>
                </tr>
              ) : (
                logs.map((log: AuditLog) => (
                  <tr key={log.id} className="hover:bg-gray-50">
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {formatDateTime(log.created_at)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium text-gray-900">{log.action}</div>
                      {log.error_msg && (
                        <div className="text-sm text-gray-500 truncate max-w-xs">{log.error_msg}</div>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {log.actor_email || log.actor_id || t('system')}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm text-gray-900">{log.resource_type}</div>
                      <div className="text-sm text-gray-500 truncate max-w-xs">{log.resource_id}</div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {getSeverityBadge(log.severity)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {log.success ? (
                        <span className="flex items-center text-green-600">
                          <CheckCircleIcon className="h-5 w-5 mr-1" />
                          {t('success')}
                        </span>
                      ) : (
                        <span className="flex items-center text-red-600">
                          <XCircleIcon className="h-5 w-5 mr-1" />
                          {t('failed')}
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {log.ip_address || '-'}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {totalPages > 1 && (
          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            totalItems={totalLogs}
            itemsPerPage={pageSize}
            onPageChange={setCurrentPage}
          />
        )}
      </Card>
    </div>
  )
}

export default AuditLogsPage
