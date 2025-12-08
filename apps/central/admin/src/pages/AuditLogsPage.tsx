import React, { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
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
  Input,
  Pagination,
} from '@/components/ui'
import { useTenantStore } from '@/stores/tenant'

const AuditLogsPage: React.FC = () => {
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

  const logs = data?.data.logs || []
  const totalLogs = data?.data.total || 0
  const totalPages = Math.ceil(totalLogs / pageSize)
  const summary = summaryData?.data.summary

  const getSeverityBadge = (severity: string) => {
    switch (severity) {
      case 'critical':
        return <Badge variant="danger">Critical</Badge>
      case 'error':
        return <Badge variant="danger">Error</Badge>
      case 'warning':
        return <Badge variant="warning">Warning</Badge>
      default:
        return <Badge variant="info">Info</Badge>
    }
  }

  // Show message if no tenant selected
  if (!selectedTenantId) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Audit Logs</h1>
          <p className="mt-2 text-sm text-gray-700">Monitor and review activity logs.</p>
        </div>
        <Card className="p-8">
          <EmptyState
            icon={<BuildingOfficeIcon className="h-12 w-12" />}
            title="No tenant selected"
            description="Please select a tenant from the header to view audit logs."
          />
        </Card>
      </div>
    )
  }

  if (isLoading) {
    return <Loading message="Loading audit logs..." />
  }

  if (error) {
    return (
      <div className="text-center py-12">
        <p className="text-red-600">Failed to load audit logs.</p>
        <Button className="mt-4" onClick={() => refetch()}>Retry</Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="sm:flex sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Audit Logs</h1>
          <p className="mt-2 text-sm text-gray-700">
            Activity logs for <span className="font-medium">{selectedTenant?.name}</span>.
          </p>
        </div>
        <Button variant="secondary" onClick={() => setShowFilters(!showFilters)}>
          <FunnelIcon className="h-5 w-5 mr-2" />
          Filters
        </Button>
      </div>

      {/* Summary Cards */}
      {summary && (
        <div className="grid grid-cols-1 md:grid-cols-5 gap-4">
          <Card className="p-4">
            <div className="text-sm text-gray-500">Last 24h</div>
            <div className="text-2xl font-bold text-gray-900">{summary.total_24h}</div>
          </Card>
          <Card className="p-4">
            <div className="text-sm text-gray-500">Last 7 days</div>
            <div className="text-2xl font-bold text-gray-900">{summary.total_7d}</div>
          </Card>
          <Card className="p-4">
            <div className="text-sm text-gray-500">Last 30 days</div>
            <div className="text-2xl font-bold text-gray-900">{summary.total_30d}</div>
          </Card>
          <Card className="p-4">
            <div className="text-sm text-gray-500">Security Events</div>
            <div className="text-2xl font-bold text-yellow-600">{summary.security_events}</div>
          </Card>
          <Card className="p-4">
            <div className="text-sm text-gray-500">Failed Operations</div>
            <div className="text-2xl font-bold text-red-600">{summary.failed_operations}</div>
          </Card>
        </div>
      )}

      {/* Filters */}
      {showFilters && (
        <Card className="p-4">
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700">Action</label>
              <Input
                value={filters.action}
                onChange={(e) => setFilters({ ...filters, action: e.target.value })}
                placeholder="e.g., user.login"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">Severity</label>
              <select
                value={filters.severity}
                onChange={(e) => setFilters({ ...filters, severity: e.target.value })}
                className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm"
              >
                <option value="">All</option>
                <option value="info">Info</option>
                <option value="warning">Warning</option>
                <option value="error">Error</option>
                <option value="critical">Critical</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">Status</label>
              <select
                value={filters.success}
                onChange={(e) => setFilters({ ...filters, success: e.target.value })}
                className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm"
              >
                <option value="">All</option>
                <option value="true">Success</option>
                <option value="false">Failed</option>
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
                Clear Filters
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
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Time</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Action</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actor</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Resource</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Severity</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">IP Address</th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {logs.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-6 py-12">
                    <EmptyState
                      icon={<ClipboardDocumentListIcon className="h-12 w-12" />}
                      title="No audit logs"
                      description="Activity will appear here as events occur"
                    />
                  </td>
                </tr>
              ) : (
                logs.map((log: AuditLog) => (
                  <tr key={log.id} className="hover:bg-gray-50">
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {new Date(log.created_at).toLocaleString()}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium text-gray-900">{log.action}</div>
                      {log.description && (
                        <div className="text-sm text-gray-500 truncate max-w-xs">{log.description}</div>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {log.actor_email || log.actor_id || 'System'}
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
                          Success
                        </span>
                      ) : (
                        <span className="flex items-center text-red-600">
                          <XCircleIcon className="h-5 w-5 mr-1" />
                          Failed
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
