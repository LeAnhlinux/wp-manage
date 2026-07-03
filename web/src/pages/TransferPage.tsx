import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  ArrowRightLeft, Plus, Eye, Loader2, CheckCircle2, AlertCircle,
  Clock, XCircle, RefreshCw, Server, Trash2,
} from 'lucide-react'
import { useTransferJobs, useDeleteTransferJob } from '@/hooks/useTransfer'
import type { TransferJob } from '@/types'

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString('vi-VN', {
    day: '2-digit', month: '2-digit', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}

const STATUS_CONFIG: Record<string, { label: string; classes: string; icon: React.ReactNode }> = {
  pending: {
    label: 'Pending',
    classes: 'bg-gray-100 text-gray-700',
    icon: <Clock size={11} />,
  },
  discovering: {
    label: 'Scanning',
    classes: 'bg-blue-100 text-blue-700',
    icon: <Loader2 size={11} className="animate-spin" />,
  },
  awaiting_selection: {
    label: 'Awaiting selection',
    classes: 'bg-amber-100 text-amber-700',
    icon: <RefreshCw size={11} />,
  },
  transferring: {
    label: 'Transferring',
    classes: 'bg-blue-100 text-blue-700',
    icon: <Loader2 size={11} className="animate-spin" />,
  },
  completed: {
    label: 'Completed',
    classes: 'bg-emerald-100 text-emerald-700',
    icon: <CheckCircle2 size={11} />,
  },
  failed: {
    label: 'Failed',
    classes: 'bg-red-100 text-red-700',
    icon: <AlertCircle size={11} />,
  },
  cancelled: {
    label: 'Cancelled',
    classes: 'bg-gray-100 text-gray-500',
    icon: <XCircle size={11} />,
  },
}

const DELETABLE = new Set(['pending', 'awaiting_selection', 'failed', 'cancelled', 'completed'])

function StatusBadge({ status }: { status: string }) {
  const cfg = STATUS_CONFIG[status] ?? {
    label: status,
    classes: 'bg-gray-100 text-gray-600',
    icon: null,
  }
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${cfg.classes}`}>
      {cfg.icon}
      {cfg.label}
    </span>
  )
}

const PANEL_LABELS: Record<string, string> = {
  aapanel: 'aaPanel',
  cpanel: 'cPanel',
  plesk: 'Plesk',
  cyberpanel: 'CyberPanel',
  directadmin: 'DirectAdmin',
}

function JobRow({
  job, onView, onDelete,
}: {
  job: TransferJob
  onView: () => void
  onDelete: () => void
}) {
  return (
    <tr className="hover:bg-gray-50/60 transition-colors">
      <td className="px-5 py-3.5 text-sm font-mono text-gray-400">#{job.id}</td>
      <td className="px-5 py-3.5">
        <p className="text-sm font-semibold text-gray-800 font-mono">{job.source_host}</p>
        <p className="text-xs text-gray-400 mt-0.5">
          {PANEL_LABELS[job.panel_type] ?? job.panel_type} · port {job.source_port}
        </p>
      </td>
      <td className="px-5 py-3.5">
        {job.target_server ? (
          <div className="flex items-center gap-1.5">
            <Server size={13} className="text-gray-300" />
            <span className="text-sm text-gray-700">{job.target_server.name}</span>
            <span className="text-xs text-gray-400 font-mono">({job.target_server.ip_address})</span>
          </div>
        ) : (
          <span className="text-sm text-gray-400">Server #{job.target_server_id}</span>
        )}
      </td>
      <td className="px-5 py-3.5">
        <StatusBadge status={job.status} />
      </td>
      <td className="px-5 py-3.5 text-sm text-gray-400">
        {(job.status === 'transferring' || job.status === 'completed') && (
          <span className="text-xs text-gray-500">
            {job.done_sites}/{job.total_sites} sites
          </span>
        )}
      </td>
      <td className="px-5 py-3.5 text-sm text-gray-400">
        <div className="flex items-center gap-1">
          <Clock size={12} />
          {formatDate(job.created_at)}
        </div>
      </td>
      <td className="px-5 py-3.5 text-right">
        <div className="flex items-center gap-2 justify-end">
          <button
            onClick={onView}
            title="View details"
            className="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium text-blue-600 bg-blue-50 hover:bg-blue-100 rounded-lg transition-colors"
          >
            <Eye size={13} /> View
          </button>
          {DELETABLE.has(job.status) && (
            <button
              onClick={onDelete}
              title="Delete job"
              className="inline-flex items-center justify-center w-7 h-7 text-gray-400 hover:text-red-600 hover:bg-red-50 rounded-lg transition-colors"
            >
              <Trash2 size={14} />
            </button>
          )}
        </div>
      </td>
    </tr>
  )
}

function JobCard({
  job, onView, onDelete,
}: {
  job: TransferJob
  onView: () => void
  onDelete: () => void
}) {
  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
      <div className="flex items-start justify-between mb-2">
        <div>
          <p className="font-semibold text-gray-800 text-sm font-mono">{job.source_host}</p>
          <p className="text-xs text-gray-400 mt-0.5">
            {PANEL_LABELS[job.panel_type] ?? job.panel_type}
          </p>
        </div>
        <StatusBadge status={job.status} />
      </div>
      {job.target_server && (
        <div className="flex items-center gap-1 text-xs text-gray-500 mb-3">
          <Server size={11} />
          {job.target_server.name} ({job.target_server.ip_address})
        </div>
      )}
      <div className="flex items-center justify-between">
        <span className="text-xs text-gray-400 flex items-center gap-1">
          <Clock size={11} />
          {formatDate(job.created_at)}
        </span>
        <div className="flex items-center gap-2">
          <button
            onClick={onView}
            className="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium text-blue-600 bg-blue-50 hover:bg-blue-100 rounded-lg transition-colors"
          >
            <Eye size={12} /> View
          </button>
          {DELETABLE.has(job.status) && (
            <button
              onClick={onDelete}
              className="inline-flex items-center justify-center w-7 h-7 text-gray-400 hover:text-red-600 hover:bg-red-50 rounded-lg transition-colors"
            >
              <Trash2 size={13} />
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

export default function TransferPage() {
  const navigate = useNavigate()
  const [page, setPage] = useState(1)
  const { data, isLoading } = useTransferJobs(page)
  const deleteJob = useDeleteTransferJob()

  const [confirmId, setConfirmId] = useState<number | null>(null)

  const jobs: TransferJob[] = data?.data ?? []
  const meta = data?.meta

  const handleConfirmDelete = async () => {
    if (!confirmId) return
    await deleteJob.mutateAsync(confirmId)
    setConfirmId(null)
  }

  return (
    <div>
      {/* Delete confirm modal */}
      {confirmId !== null && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="bg-white rounded-2xl shadow-xl p-6 w-full max-w-sm mx-4">
            <div className="flex items-center gap-3 mb-3">
              <div className="w-10 h-10 rounded-full bg-red-100 flex items-center justify-center flex-shrink-0">
                <Trash2 size={18} className="text-red-600" />
              </div>
              <h3 className="text-base font-semibold text-gray-900">Delete transfer job #{confirmId}?</h3>
            </div>
            <p className="text-sm text-gray-500 mb-5">
              The job and all its results will be permanently deleted. This cannot be undone.
            </p>
            <div className="flex gap-3 justify-end">
              <button
                onClick={() => setConfirmId(null)}
                className="px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 rounded-xl transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleConfirmDelete}
                disabled={deleteJob.isPending}
                className="flex items-center gap-2 px-4 py-2 text-sm font-semibold bg-red-600 text-white hover:bg-red-700 rounded-xl disabled:opacity-50 transition-colors"
              >
                {deleteJob.isPending ? <Loader2 size={14} className="animate-spin" /> : <Trash2 size={14} />}
                Delete
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between mb-7">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 leading-none flex items-center gap-2">
            <ArrowRightLeft size={22} className="text-blue-500" />
            Transfer WordPress Sites
          </h1>
          <p className="text-gray-500 text-sm mt-1">
            Import all sites from a cPanel / aaPanel / Plesk server into this system
          </p>
        </div>
        <button
          onClick={() => navigate('/transfer/new')}
          className="flex items-center gap-2 px-4 py-2.5 bg-blue-600 text-white rounded-xl hover:bg-blue-700 text-sm font-semibold shadow-sm shadow-blue-200 transition-colors"
        >
          <Plus size={15} /> Start new transfer
        </button>
      </div>

      {isLoading ? (
        <div className="bg-white rounded-2xl border border-gray-100 p-12 text-center">
          <Loader2 size={28} className="animate-spin text-gray-300 mx-auto" />
        </div>
      ) : jobs.length === 0 ? (
        <div className="bg-white rounded-2xl border border-gray-100 p-16 text-center">
          <ArrowRightLeft size={44} className="mx-auto text-gray-200 mb-4" />
          <p className="text-gray-600 font-semibold">No transfers yet</p>
          <p className="text-gray-400 text-sm mt-1 mb-6">
            Click "+ Start new transfer" to import sites from another panel
          </p>
          <button
            onClick={() => navigate('/transfer/new')}
            className="inline-flex items-center gap-2 px-5 py-2.5 bg-blue-600 text-white rounded-xl hover:bg-blue-700 text-sm font-semibold transition-colors"
          >
            <Plus size={15} /> Start new transfer
          </button>
        </div>
      ) : (
        <>
          {/* Desktop table */}
          <div className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden hidden md:block">
            <table className="w-full">
              <thead>
                <tr className="border-b border-gray-50">
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">ID</th>
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Source</th>
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Target Server</th>
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Status</th>
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Progress</th>
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Time</th>
                  <th className="text-right px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-50">
                {jobs.map((job) => (
                  <JobRow
                    key={job.id}
                    job={job}
                    onView={() => navigate(`/transfer/${job.id}`)}
                    onDelete={() => setConfirmId(job.id)}
                  />
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile cards */}
          <div className="space-y-3 md:hidden">
            {jobs.map((job) => (
              <JobCard
                key={job.id}
                job={job}
                onView={() => navigate(`/transfer/${job.id}`)}
                onDelete={() => setConfirmId(job.id)}
              />
            ))}
          </div>

          {/* Pagination */}
          {meta && meta.total_pages > 1 && (
            <div className="flex items-center justify-between mt-5">
              <p className="text-sm text-gray-500">
                Page {meta.page} / {meta.total_pages} ({meta.total} jobs)
              </p>
              <div className="flex gap-2">
                <button
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page <= 1}
                  className="px-3 py-1.5 border border-gray-200 rounded-xl text-sm hover:bg-gray-50 disabled:opacity-40 transition-colors"
                >
                  Previous
                </button>
                <button
                  onClick={() => setPage((p) => p + 1)}
                  disabled={page >= meta.total_pages}
                  className="px-3 py-1.5 border border-gray-200 rounded-xl text-sm hover:bg-gray-50 disabled:opacity-40 transition-colors"
                >
                  Next
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}
