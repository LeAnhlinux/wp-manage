import { useState } from 'react'
import { Loader2, Shield, CheckCircle2 } from 'lucide-react'
import { useWAFConfig, useUpdateWAFConfig } from '@/hooks/useSecurity'

export function WAFPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: waf, isLoading } = useWAFConfig(siteId)
  const updateWAF = useUpdateWAFConfig()

  const [enabled, setEnabled] = useState<boolean | null>(null)
  const [paranoidLevel, setParanoidLevel] = useState<number | null>(null)
  const [whitelistedIPs, setWhitelistedIPs] = useState<string | null>(null)
  const [saveMsg, setSaveMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  // Sync from fetched data (only first time)
  const effectiveEnabled = enabled !== null ? enabled : (waf?.enabled ?? false)
  const effectiveLevel = paranoidLevel !== null ? paranoidLevel : (waf?.paranoid_level ?? 1)
  const effectiveIPs = whitelistedIPs !== null ? whitelistedIPs : (waf?.whitelisted_ips ?? '')

  const paranoidLevels = [
    { level: 1, label: 'Level 1 — Basic', desc: 'Minimal rules, low false-positive rate. Good starting point.' },
    { level: 2, label: 'Level 2 — Normal', desc: 'More rules enabled. Balanced protection for most sites.' },
    { level: 3, label: 'Level 3 — Strict', desc: 'Aggressive rules. May require IP whitelisting for some tools.' },
    { level: 4, label: 'Level 4 — Maximum', desc: 'All OWASP CRS rules active. Highest security, highest false positives.' },
  ]

  const handleSave = async () => {
    setSaveMsg(null)
    try {
      await updateWAF.mutateAsync({
        siteId,
        enabled: effectiveEnabled,
        paranoid_level: effectiveLevel,
        whitelisted_ips: effectiveIPs,
      })
      setSaveMsg({ type: 'success', text: 'WAF configuration saved successfully.' })
      // Reset local overrides so we display from server next render
      setEnabled(null)
      setParanoidLevel(null)
      setWhitelistedIPs(null)
    } catch {
      setSaveMsg({ type: 'error', text: 'Failed to save WAF configuration.' })
    }
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-2">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
          <Shield size={20} className="text-blue-600" /> ModSecurity WAF
        </h2>
        {isLoading && <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-blue-600" />}
      </div>
      <p className="text-sm text-gray-500 mb-6">
        OWASP Core Rule Set (CRS) — blocks SQL injection, XSS, remote code execution, and other common web attacks.
      </p>

      {/* Enable / Disable toggle */}
      <div className={`flex items-center justify-between p-4 rounded-lg border mb-5 ${
        effectiveEnabled ? 'border-blue-200 bg-blue-50' : 'border-gray-200 bg-gray-50'
      }`}>
        <div>
          <p className="text-sm font-medium text-gray-900">WAF Protection</p>
          <p className="text-xs text-gray-500 mt-0.5">
            {effectiveEnabled ? 'ModSecurity is active and blocking threats' : 'ModSecurity is disabled — site is unprotected'}
          </p>
        </div>
        <button
          onClick={() => setEnabled(!effectiveEnabled)}
          disabled={!canManage}
          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none ${
            effectiveEnabled ? 'bg-blue-600' : 'bg-gray-300'
          } disabled:opacity-50`}
        >
          <span className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
            effectiveEnabled ? 'translate-x-6' : 'translate-x-1'
          }`} />
        </button>
      </div>

      {/* Paranoid Level */}
      {effectiveEnabled && (
        <>
          <div className="mb-5">
            <p className="text-sm font-medium text-gray-900 mb-3">Paranoid Level</p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
              {paranoidLevels.map(pl => (
                <button
                  key={pl.level}
                  onClick={() => setParanoidLevel(pl.level)}
                  disabled={!canManage}
                  className={`text-left p-3 rounded-lg border transition-colors ${
                    effectiveLevel === pl.level
                      ? 'border-blue-500 bg-blue-50'
                      : 'border-gray-200 hover:border-gray-300 hover:bg-gray-50'
                  } disabled:opacity-50`}
                >
                  <p className={`text-sm font-medium ${effectiveLevel === pl.level ? 'text-blue-700' : 'text-gray-800'}`}>
                    {pl.label}
                  </p>
                  <p className="text-xs text-gray-500 mt-0.5">{pl.desc}</p>
                </button>
              ))}
            </div>
          </div>

          {/* Whitelisted IPs */}
          <div className="mb-5">
            <label className="block text-sm font-medium text-gray-900 mb-1">
              Whitelisted IPs
            </label>
            <p className="text-xs text-gray-500 mb-2">
              One IP per line. These IPs bypass WAF rules (useful for your office IP or monitoring services).
            </p>
            <textarea
              value={effectiveIPs}
              onChange={e => setWhitelistedIPs(e.target.value)}
              disabled={!canManage}
              rows={4}
              placeholder={"192.168.1.1\n10.0.0.0/8"}
              className="w-full text-sm font-mono px-3 py-2 border border-gray-200 rounded-lg bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none resize-none disabled:opacity-50"
            />
          </div>
        </>
      )}

      {/* Rule set info */}
      {waf?.rule_set && (
        <div className="flex items-center gap-2 text-xs text-gray-500 mb-5">
          <CheckCircle2 size={14} className="text-green-500" />
          <span>Active rule set: <span className="font-medium text-gray-700">{waf.rule_set}</span></span>
        </div>
      )}

      {canManage && (
        <button
          onClick={handleSave}
          disabled={updateWAF.isPending}
          className="flex items-center gap-2 px-4 py-2.5 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
        >
          {updateWAF.isPending ? <><Loader2 size={15} className="animate-spin" /> Applying on server…</> : <><Shield size={15} /> Save WAF Config</>}
        </button>
      )}

      {saveMsg && (
        <div className={`mt-4 p-3 rounded-lg border text-sm ${
          saveMsg.type === 'success' ? 'bg-green-50 border-green-200 text-green-700' : 'bg-red-50 border-red-200 text-red-700'
        }`}>
          {saveMsg.text}
        </div>
      )}
    </div>
  )
}
