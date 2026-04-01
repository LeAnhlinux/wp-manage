import { useState } from 'react'
import { Loader2, Shield } from 'lucide-react'
import { useHardenWordPress, useRevertHardening } from '@/hooks/useHardening'

type HardenOptionsState = {
  disable_xmlrpc: boolean
  hide_wp_version: boolean
  secure_file_perms: boolean
  security_headers: boolean
  disable_file_editor: boolean
  limit_login_attempts: boolean
}

export const HARDEN_OPTIONS_LIST = [
  { key: 'disable_xmlrpc' as const, label: 'Disable XML-RPC', desc: 'Block xmlrpc.php to prevent brute-force and DDoS attacks' },
  { key: 'hide_wp_version' as const, label: 'Hide WordPress Version', desc: 'Remove WP version meta tag from page source' },
  { key: 'secure_file_perms' as const, label: 'Secure File Permissions', desc: 'Set directories to 755, files to 644, wp-config.php to 600' },
  { key: 'security_headers' as const, label: 'Add Security Headers', desc: 'Add X-Content-Type-Options, X-Frame-Options, XSS-Protection headers' },
  { key: 'disable_file_editor' as const, label: 'Disable File Editor', desc: 'Disable the built-in WordPress theme/plugin editor' },
  { key: 'limit_login_attempts' as const, label: 'Limit Login Attempts', desc: 'Rate limit wp-login.php to 5 requests/min per IP + Fail2ban auto-ban' },
]

export function SecurityPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const harden = useHardenWordPress(siteId)
  const revert = useRevertHardening(siteId)

  const defaultOptions: HardenOptionsState = {
    disable_xmlrpc: true,
    hide_wp_version: true,
    secure_file_perms: true,
    security_headers: true,
    disable_file_editor: true,
    limit_login_attempts: true,
  }

  const [applyOptions, setApplyOptions] = useState<HardenOptionsState>(defaultOptions)
  const [revertOptions, setRevertOptions] = useState<HardenOptionsState>(defaultOptions)
  const [activeAction, setActiveAction] = useState<'apply' | 'revert'>('apply')

  const [applyMsg, setApplyMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [revertMsg, setRevertMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const handleApply = async () => {
    if (!confirm('Apply security hardening? This will modify server configurations.')) return
    setApplyMsg(null)
    try {
      await harden.mutateAsync(applyOptions)
      setApplyMsg({ type: 'success', text: '✓ Hardening applied successfully. All selected measures are now active on the server.' })
    } catch (err: any) {
      setApplyMsg({ type: 'error', text: err?.response?.data?.error || 'Failed to apply hardening. Please try again.' })
    }
  }

  const handleRevert = async () => {
    if (!confirm('Revert selected hardening measures? This will undo the selected security configurations.')) return
    setRevertMsg(null)
    try {
      await revert.mutateAsync(revertOptions)
      setRevertMsg({ type: 'success', text: '✓ Hardening measures reverted successfully.' })
    } catch (err: any) {
      setRevertMsg({ type: 'error', text: err?.response?.data?.error || 'Failed to revert hardening. Please try again.' })
    }
  }

  const toggleApply = (key: keyof HardenOptionsState) =>
    setApplyOptions(prev => ({ ...prev, [key]: !prev[key] }))

  const toggleRevert = (key: keyof HardenOptionsState) =>
    setRevertOptions(prev => ({ ...prev, [key]: !prev[key] }))

  const busy = harden.isPending || revert.isPending

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-4">
        <Shield size={20} className="text-green-600" /> WordPress Security Hardening
      </h2>

      {/* Tab switcher */}
      <div className="flex gap-1 mb-6 bg-gray-100 rounded-lg p-1 w-fit">
        <button
          onClick={() => setActiveAction('apply')}
          className={`px-4 py-1.5 text-sm font-medium rounded-md transition-colors ${
            activeAction === 'apply' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-700'
          }`}
        >
          Apply
        </button>
        <button
          onClick={() => setActiveAction('revert')}
          className={`px-4 py-1.5 text-sm font-medium rounded-md transition-colors ${
            activeAction === 'revert' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-700'
          }`}
        >
          Revert
        </button>
      </div>

      {activeAction === 'apply' ? (
        <>
          <p className="text-sm text-gray-500 mb-5">
            Select the hardening measures to apply to this site, then click <strong>Apply Hardening</strong>.
          </p>

          <div className="space-y-3 mb-6">
            {HARDEN_OPTIONS_LIST.map(opt => (
              <label key={opt.key}
                className={`flex items-start gap-3 p-4 border rounded-lg cursor-pointer transition-colors ${
                  applyOptions[opt.key] ? 'border-green-200 bg-green-50' : 'border-gray-200 hover:bg-gray-50'
                }`}>
                <input type="checkbox" checked={applyOptions[opt.key]} onChange={() => toggleApply(opt.key)}
                  className="mt-0.5 h-4 w-4 text-green-600 rounded border-gray-300 focus:ring-green-500" />
                <div>
                  <p className="text-sm font-medium text-gray-900">{opt.label}</p>
                  <p className="text-xs text-gray-500 mt-0.5">{opt.desc}</p>
                </div>
              </label>
            ))}
          </div>

          {canManage && (
            <button
              onClick={handleApply}
              disabled={busy || !Object.values(applyOptions).some(v => v)}
              className="flex items-center gap-2 px-4 py-2.5 text-sm text-white bg-green-600 rounded-lg hover:bg-green-700 disabled:opacity-50"
            >
              {harden.isPending ? <><Loader2 size={16} className="animate-spin" /> Applying on server…</> : <><Shield size={16} /> Apply Hardening</>}
            </button>
          )}

          {applyMsg && (
            <div className={`mt-4 p-3 rounded-lg border text-sm ${
              applyMsg.type === 'success' ? 'bg-green-50 border-green-200 text-green-700' : 'bg-red-50 border-red-200 text-red-700'
            }`}>
              {applyMsg.text}
            </div>
          )}
        </>
      ) : (
        <>
          <p className="text-sm text-gray-500 mb-5">
            Select which hardening measures to <strong>undo</strong>. Only measures that were previously applied will be affected.
            <br /><span className="text-amber-600 font-medium">Note: File permissions (755/644/600) are not reverted as they are safe to keep.</span>
          </p>

          <div className="space-y-3 mb-6">
            {HARDEN_OPTIONS_LIST.filter(o => o.key !== 'secure_file_perms').map(opt => (
              <label key={opt.key}
                className={`flex items-start gap-3 p-4 border rounded-lg cursor-pointer transition-colors ${
                  revertOptions[opt.key] ? 'border-amber-200 bg-amber-50' : 'border-gray-200 hover:bg-gray-50'
                }`}>
                <input type="checkbox" checked={revertOptions[opt.key]} onChange={() => toggleRevert(opt.key)}
                  className="mt-0.5 h-4 w-4 text-amber-600 rounded border-gray-300 focus:ring-amber-500" />
                <div>
                  <p className="text-sm font-medium text-gray-900">{opt.label}</p>
                  <p className="text-xs text-gray-500 mt-0.5">{opt.desc}</p>
                </div>
              </label>
            ))}
            {/* Secure file perms - show as disabled */}
            <div className="flex items-start gap-3 p-4 border border-gray-100 rounded-lg opacity-50 cursor-not-allowed bg-gray-50">
              <input type="checkbox" disabled checked={false}
                className="mt-0.5 h-4 w-4 rounded border-gray-300" />
              <div>
                <p className="text-sm font-medium text-gray-500">Secure File Permissions</p>
                <p className="text-xs text-gray-400 mt-0.5">Cannot be reverted — 755/644/600 permissions are safe to keep</p>
              </div>
            </div>
          </div>

          {canManage && (
            <button
              onClick={handleRevert}
              disabled={busy || !HARDEN_OPTIONS_LIST.filter(o => o.key !== 'secure_file_perms').some(o => revertOptions[o.key])}
              className="flex items-center gap-2 px-4 py-2.5 text-sm text-white bg-amber-600 rounded-lg hover:bg-amber-700 disabled:opacity-50"
            >
              {revert.isPending ? <><Loader2 size={16} className="animate-spin" /> Reverting on server…</> : <><Shield size={16} /> Revert Hardening</>}
            </button>
          )}

          {revertMsg && (
            <div className={`mt-4 p-3 rounded-lg border text-sm ${
              revertMsg.type === 'success' ? 'bg-green-50 border-green-200 text-green-700' : 'bg-red-50 border-red-200 text-red-700'
            }`}>
              {revertMsg.text}
            </div>
          )}
        </>
      )}
    </div>
  )
}
