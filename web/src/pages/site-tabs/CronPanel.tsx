import { Clock } from 'lucide-react'
import { useCronJobsBySite } from '@/hooks/useCronJobs'

export function CronPanel({ siteId }: { siteId: number }) {
  const { data: jobs = [], isLoading } = useCronJobsBySite(siteId)

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-4">
        <Clock size={20} className="text-green-600" /> Cron Jobs
      </h2>
      <p className="text-sm text-gray-500 mb-4">
        Cron jobs associated with this site. Manage all cron jobs from the server detail page.
      </p>

      {isLoading ? (
        <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
      ) : jobs.length === 0 ? (
        <p className="text-sm text-gray-400 text-center py-8">No cron jobs for this site</p>
      ) : (
        <div className="space-y-3">
          {jobs.map(job => (
            <div key={job.id} className="p-3 border border-gray-200 rounded-lg">
              <div className="flex items-center gap-2 mb-1">
                <code className="text-xs bg-gray-100 px-2 py-0.5 rounded font-mono">{job.schedule}</code>
                <span className={`px-2 py-0.5 rounded text-xs ${job.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'}`}>
                  {job.enabled ? 'Active' : 'Disabled'}
                </span>
              </div>
              <p className="text-sm font-mono text-gray-700">{job.command}</p>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
