import { useState, useEffect, useRef } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import { Key, Plus, Trash2, Copy, Check, ShieldCheck, ShieldOff, Bell, Save, Settings, Loader2, Download, RefreshCw, CheckCircle, XCircle, AlertTriangle, Server } from 'lucide-react'
import { useAPIKeys, useCreateAPIKey, useDeleteAPIKey } from '@/hooks/useAPIKeys'
import { useEnable2FA, useVerify2FA, useDisable2FA } from '@/hooks/useTwoFactor'
import { useNotificationConfig, useUpdateNotificationConfig, useIntegrationConfig, useUpdateIntegrationConfig } from '@/hooks/useAlerts'
import { useLastUpdateResult, useCheckUpdate, useApplyPanelUpdate, useApplyAgentUpdate, type CheckUpdateResult } from '@/hooks/usePanelUpdate'
import { useRollout } from '@/hooks/useRollouts'
import { useQueryClient } from '@tanstack/react-query'
import api from '@/lib/api'
import { useAuth } from '@/stores/auth'

export default function SettingsPage() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Settings</h1>
      <div className="space-y-6">
        <TwoFactorSection />
        <NotificationSection />
        {isAdmin && <IntegrationsSection />}
        <APIKeysSection />
        {isAdmin && <UpdatesSection />}
      </div>
    </div>
  )
}

function TwoFactorSection() {
  const { user, loadUser } = useAuth()
  const [step, setStep] = useState<'idle' | 'setup' | 'verify'>('idle')
  const [verifyCode, setVerifyCode] = useState('')
  const [setupData, setSetupData] = useState<{ secret: string; qr_code_uri: string; recovery_codes: string[] } | null>(null)
  const [copiedCodes, setCopiedCodes] = useState(false)

  const enable2FA = useEnable2FA()
  const verify2FA = useVerify2FA()
  const disable2FA = useDisable2FA()

  const handleEnable = async () => {
    try {
      const data = await enable2FA.mutateAsync()
      setSetupData(data)
      setStep('setup')
    } catch {}
  }

  const handleVerify = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await verify2FA.mutateAsync(verifyCode)
      setStep('idle')
      setSetupData(null)
      setVerifyCode('')
      loadUser()
    } catch {}
  }

  const handleDisable = async () => {
    if (!confirm('Are you sure you want to disable 2FA? This will make your account less secure.')) return
    try {
      await disable2FA.mutateAsync()
      loadUser()
    } catch {}
  }

  const copyRecoveryCodes = () => {
    if (setupData?.recovery_codes) {
      navigator.clipboard.writeText(setupData.recovery_codes.join('\n'))
      setCopiedCodes(true)
      setTimeout(() => setCopiedCodes(false), 2000)
    }
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <ShieldCheck size={20} className="text-gray-600" />
          <h2 className="text-lg font-semibold">Two-Factor Authentication</h2>
        </div>
        {user?.two_factor_enabled ? (
          <span className="px-2.5 py-1 text-xs font-medium bg-green-100 text-green-700 rounded-full">Enabled</span>
        ) : (
          <span className="px-2.5 py-1 text-xs font-medium bg-gray-100 text-gray-500 rounded-full">Disabled</span>
        )}
      </div>

      <p className="text-sm text-gray-500 mb-4">
        Add an extra layer of security to your account by requiring a verification code from your authenticator app when signing in.
      </p>

      {step === 'idle' && !user?.two_factor_enabled && (
        <button onClick={handleEnable} disabled={enable2FA.isPending}
          className="flex items-center gap-2 px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
          <ShieldCheck size={16} />
          {enable2FA.isPending ? 'Setting up...' : 'Enable 2FA'}
        </button>
      )}

      {step === 'idle' && user?.two_factor_enabled && (
        <button onClick={handleDisable} disabled={disable2FA.isPending}
          className="flex items-center gap-2 px-4 py-2 text-sm bg-red-600 text-white rounded-lg hover:bg-red-700 disabled:opacity-50">
          <ShieldOff size={16} />
          {disable2FA.isPending ? 'Disabling...' : 'Disable 2FA'}
        </button>
      )}

      {step === 'setup' && setupData && (
        <div className="space-y-4">
          <div className="bg-blue-50 border border-blue-200 rounded-lg p-4">
            <h3 className="font-medium text-blue-800 mb-2">Step 1: Scan QR Code</h3>
            <p className="text-sm text-blue-700 mb-3">
              Open your authenticator app (Google Authenticator, Authy, etc.) and scan this QR code, or enter the secret key manually.
            </p>
            <div className="flex items-center gap-4">
              <div className="w-48 h-48 border rounded-lg bg-white p-2 flex items-center justify-center">
                <QRCodeSVG value={setupData.qr_code_uri} size={176} />
              </div>
              <div>
                <p className="text-xs text-gray-500 mb-1">Secret Key:</p>
                <code className="text-sm font-mono bg-white px-3 py-1.5 rounded border break-all block">{setupData.secret}</code>
              </div>
            </div>
          </div>

          <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
            <h3 className="font-medium text-yellow-800 mb-2">Step 2: Save Recovery Codes</h3>
            <p className="text-sm text-yellow-700 mb-3">
              Save these recovery codes in a safe place. You can use them to access your account if you lose your authenticator device.
            </p>
            <div className="grid grid-cols-2 gap-1 mb-3">
              {setupData.recovery_codes.map((code, i) => (
                <code key={i} className="text-sm font-mono bg-white px-2 py-1 rounded border">{code}</code>
              ))}
            </div>
            <button onClick={copyRecoveryCodes}
              className="flex items-center gap-1 text-sm text-yellow-700 hover:text-yellow-800">
              {copiedCodes ? <Check size={14} /> : <Copy size={14} />}
              {copiedCodes ? 'Copied!' : 'Copy all codes'}
            </button>
          </div>

          <div>
            <button onClick={() => setStep('verify')}
              className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700">
              I've saved my codes → Verify
            </button>
          </div>
        </div>
      )}

      {step === 'verify' && (
        <form onSubmit={handleVerify} className="space-y-3 max-w-sm">
          <h3 className="font-medium">Step 3: Verify Code</h3>
          <p className="text-sm text-gray-500">Enter the 6-digit code from your authenticator app to activate 2FA.</p>
          <input
            type="text"
            value={verifyCode}
            onChange={(e) => setVerifyCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
            className="w-full px-4 py-2.5 border border-gray-300 rounded-lg text-center text-xl tracking-widest font-mono"
            placeholder="000000"
            maxLength={6}
            autoFocus
            required
          />
          <div className="flex gap-2">
            <button type="submit" disabled={verify2FA.isPending || verifyCode.length < 6}
              className="px-4 py-2 text-sm bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:opacity-50">
              {verify2FA.isPending ? 'Verifying...' : 'Activate 2FA'}
            </button>
            <button type="button" onClick={() => { setStep('idle'); setSetupData(null); setVerifyCode('') }}
              className="px-4 py-2 text-sm bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300">
              Cancel
            </button>
          </div>
          {verify2FA.isError && (
            <p className="text-sm text-red-600">Invalid code. Please try again.</p>
          )}
        </form>
      )}
    </div>
  )
}

function NotificationSection() {
  const { data: config } = useNotificationConfig()
  const updateConfig = useUpdateNotificationConfig()
  const [form, setForm] = useState({
    telegram_token: '', telegram_chat: '',
    smtp_host: '', smtp_port: 587, smtp_user: '', smtp_pass: '', smtp_from: '',
  })
  const [initialized, setInitialized] = useState(false)

  if (config && !initialized) {
    setForm({
      telegram_token: config.telegram_token || '',
      telegram_chat: config.telegram_chat || '',
      smtp_host: config.smtp_host || '',
      smtp_port: config.smtp_port || 587,
      smtp_user: config.smtp_user || '',
      smtp_pass: '',
      smtp_from: config.smtp_from || '',
    })
    setInitialized(true)
  }

  const handleSave = async () => {
    try {
      await updateConfig.mutateAsync(form)
    } catch {}
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-4">
        <Bell size={20} className="text-orange-500" /> Notification Settings
      </h2>
      <p className="text-sm text-gray-500 mb-4">Configure channels for uptime alerts, resource alerts, and SSL expiry notifications.</p>

      <div className="space-y-6">
        {/* Telegram */}
        <div>
          <h3 className="text-sm font-medium text-gray-700 mb-2">Telegram</h3>
          <div className="grid grid-cols-2 gap-3">
            <input value={form.telegram_token} onChange={e => setForm({ ...form, telegram_token: e.target.value })}
              className="px-3 py-2 border rounded-lg text-sm" placeholder="Bot Token" />
            <input value={form.telegram_chat} onChange={e => setForm({ ...form, telegram_chat: e.target.value })}
              className="px-3 py-2 border rounded-lg text-sm" placeholder="Chat ID" />
          </div>
        </div>

        {/* Email SMTP */}
        <div>
          <h3 className="text-sm font-medium text-gray-700 mb-2">Email (SMTP)</h3>
          <div className="grid grid-cols-2 gap-3">
            <input value={form.smtp_host} onChange={e => setForm({ ...form, smtp_host: e.target.value })}
              className="px-3 py-2 border rounded-lg text-sm" placeholder="SMTP Host" />
            <input type="number" value={form.smtp_port} onChange={e => setForm({ ...form, smtp_port: Number(e.target.value) })}
              className="px-3 py-2 border rounded-lg text-sm" placeholder="Port" />
            <input value={form.smtp_user} onChange={e => setForm({ ...form, smtp_user: e.target.value })}
              className="px-3 py-2 border rounded-lg text-sm" placeholder="Username" />
            <input type="password" value={form.smtp_pass} onChange={e => setForm({ ...form, smtp_pass: e.target.value })}
              className="px-3 py-2 border rounded-lg text-sm" placeholder="Password" />
            <input value={form.smtp_from} onChange={e => setForm({ ...form, smtp_from: e.target.value })}
              className="col-span-2 px-3 py-2 border rounded-lg text-sm" placeholder="From email" />
          </div>
        </div>
      </div>

      <button onClick={handleSave} disabled={updateConfig.isPending}
        className="mt-4 flex items-center gap-2 px-4 py-2.5 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
        <Save size={16} />
        {updateConfig.isPending ? 'Saving...' : 'Save Settings'}
      </button>

      {updateConfig.isSuccess && (
        <p className="mt-2 text-sm text-green-600">Notification settings saved successfully.</p>
      )}
    </div>
  )
}

function IntegrationsSection() {
  const { data: config, isLoading } = useIntegrationConfig()
  const update = useUpdateIntegrationConfig()
  const [key, setKey] = useState('')
  const [saved, setSaved] = useState(false)
  const [showKey, setShowKey] = useState(false)

  useEffect(() => {
    if (config) setKey(config.google_pagespeed_key || '')
  }, [config])

  const handleSave = async () => {
    await update.mutateAsync({ google_pagespeed_key: key })
    setSaved(true)
    setTimeout(() => setSaved(false), 2500)
  }

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
      <div className="flex items-center gap-3 mb-1">
        <div className="w-9 h-9 rounded-xl bg-blue-50 flex items-center justify-center">
          <Key size={18} className="text-blue-600" />
        </div>
        <div>
          <h2 className="text-base font-semibold text-gray-900">Integrations</h2>
          <p className="text-xs text-gray-500">Third-party API keys</p>
        </div>
      </div>

      <div className="mt-5 border-t border-gray-50 pt-5">
        <div className="flex items-start gap-3">
          <div className="flex-1">
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Google PageSpeed API Key
            </label>
            <p className="text-xs text-gray-400 mb-2">
              Required for Performance Metrics. Free key at{' '}
              <a href="https://console.cloud.google.com" target="_blank" rel="noopener"
                className="text-blue-500 hover:underline">console.cloud.google.com</a>
              {' '}→ PageSpeed Insights API (25,000 req/day free).
            </p>
            <div className="flex gap-2">
              <div className="relative flex-1">
                <input
                  type={showKey ? 'text' : 'password'}
                  value={isLoading ? '' : key}
                  onChange={(e) => setKey(e.target.value)}
                  placeholder={isLoading ? 'Loading...' : 'AIzaSy...'}
                  disabled={isLoading}
                  className="w-full px-4 py-2.5 border border-gray-200 rounded-xl text-sm font-mono bg-gray-50 focus:bg-white focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 outline-none transition-colors pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowKey(v => !v)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 text-xs"
                >
                  {showKey ? 'Hide' : 'Show'}
                </button>
              </div>
              <button
                onClick={handleSave}
                disabled={update.isPending || isLoading}
                className="flex items-center gap-2 px-4 py-2.5 bg-blue-600 text-white rounded-xl hover:bg-blue-700 disabled:opacity-50 text-sm font-semibold transition-colors"
              >
                {update.isPending ? <Loader2 size={14} className="animate-spin" /> : saved ? <Check size={14} /> : <Save size={14} />}
                {saved ? 'Saved!' : 'Save'}
              </button>
            </div>
            {key && (
              <div className="mt-2 flex items-center gap-1.5 text-xs text-emerald-600">
                <Check size={12} /> API key configured
              </div>
            )}
            {!key && !isLoading && (
              <div className="mt-2 flex items-center gap-1.5 text-xs text-amber-500">
                <AlertTriangle size={12} /> No key set — limited to ~25 audits/day
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function APIKeysSection() {
  const [showCreate, setShowCreate] = useState(false)
  const [newKey, setNewKey] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [form, setForm] = useState({ name: '', rate_limit: 100, expires_in: 0 })

  const { data: keys, isLoading } = useAPIKeys()
  const createKey = useCreateAPIKey()
  const deleteKey = useDeleteAPIKey()

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      const result = await createKey.mutateAsync(form)
      setNewKey(result.data?.key || null)
      setShowCreate(false)
      setForm({ name: '', rate_limit: 100, expires_in: 0 })
    } catch {}
  }

  const copyKey = () => {
    if (newKey) {
      navigator.clipboard.writeText(newKey)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <Key size={20} className="text-gray-600" />
          <h2 className="text-lg font-semibold">API Keys</h2>
        </div>
        <button onClick={() => { setShowCreate(!showCreate); setNewKey(null) }}
          className="flex items-center gap-2 px-3 py-1.5 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700">
          <Plus size={14} /> Create Key
        </button>
      </div>

      <p className="text-sm text-gray-500 mb-4">
        Use API keys to access the WP Manage API programmatically. Include the key in the <code className="bg-gray-100 px-1 py-0.5 rounded text-xs">X-API-Key</code> header.
      </p>

      {newKey && (
        <div className="bg-green-50 border border-green-200 rounded-lg p-4 mb-4">
          <p className="text-sm font-medium text-green-800 mb-2">
            API key created! Copy it now — it won't be shown again.
          </p>
          <div className="flex items-center gap-2">
            <code className="flex-1 bg-white px-3 py-2 rounded border border-green-300 text-sm font-mono break-all">
              {newKey}
            </code>
            <button onClick={copyKey}
              className="p-2 text-green-700 hover:bg-green-100 rounded-lg">
              {copied ? <Check size={18} /> : <Copy size={18} />}
            </button>
          </div>
        </div>
      )}

      {showCreate && (
        <form onSubmit={handleCreate} className="bg-gray-50 rounded-lg p-4 mb-4 space-y-3">
          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
              <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm" placeholder="My API Key" required />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Rate Limit (req/min)</label>
              <input type="number" value={form.rate_limit} onChange={(e) => setForm({ ...form, rate_limit: Number(e.target.value) })}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm" min={1} max={10000} />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Expires In (days)</label>
              <input type="number" value={form.expires_in} onChange={(e) => setForm({ ...form, expires_in: Number(e.target.value) })}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm" min={0} placeholder="0 = never" />
            </div>
          </div>
          <div className="flex gap-2">
            <button type="submit" disabled={createKey.isPending}
              className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
              {createKey.isPending ? 'Creating...' : 'Create'}
            </button>
            <button type="button" onClick={() => setShowCreate(false)}
              className="px-4 py-2 text-sm bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300">
              Cancel
            </button>
          </div>
        </form>
      )}

      <div className="border rounded-lg overflow-hidden">
        <table className="w-full">
          <thead className="bg-gray-50 border-b">
            <tr>
              <th className="text-left px-4 py-2.5 text-xs font-medium text-gray-500 uppercase">Name</th>
              <th className="text-left px-4 py-2.5 text-xs font-medium text-gray-500 uppercase">Key</th>
              <th className="text-left px-4 py-2.5 text-xs font-medium text-gray-500 uppercase">Rate Limit</th>
              <th className="text-left px-4 py-2.5 text-xs font-medium text-gray-500 uppercase">Last Used</th>
              <th className="text-left px-4 py-2.5 text-xs font-medium text-gray-500 uppercase">Expires</th>
              <th className="text-right px-4 py-2.5 text-xs font-medium text-gray-500 uppercase">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-50">
            {isLoading && <tr><td colSpan={6} className="px-4 py-6 text-center text-gray-400 text-sm">Loading...</td></tr>}
            {(keys || []).map((key) => (
              <tr key={key.id} className="hover:bg-gray-50">
                <td className="px-4 py-3 text-sm font-medium">{key.name}</td>
                <td className="px-4 py-3 text-sm font-mono text-gray-500">{key.key_prefix}••••••••</td>
                <td className="px-4 py-3 text-sm text-gray-600">{key.rate_limit}/min</td>
                <td className="px-4 py-3 text-sm text-gray-500">
                  {key.last_used_at ? new Date(key.last_used_at).toLocaleDateString() : 'Never'}
                </td>
                <td className="px-4 py-3 text-sm text-gray-500">
                  {key.expires_at ? new Date(key.expires_at).toLocaleDateString() : 'Never'}
                </td>
                <td className="px-4 py-3 text-right">
                  <button onClick={() => { if (confirm(`Delete API key "${key.name}"?`)) deleteKey.mutate(key.id) }}
                    className="p-1.5 text-gray-400 hover:text-red-600">
                    <Trash2 size={14} />
                  </button>
                </td>
              </tr>
            ))}
            {!isLoading && (!keys || keys.length === 0) && (
              <tr><td colSpan={6} className="px-4 py-6 text-center text-gray-400 text-sm">No API keys yet.</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}


// =============================================================================
// Combined Updates Section (Panel + Agent)
// =============================================================================
function UpdatesSection() {
  const checkUpdate = useCheckUpdate()
  const applyPanel = useApplyPanelUpdate()
  const applyAgents = useApplyAgentUpdate()
  const { data: lastResult } = useLastUpdateResult()
  const queryClient = useQueryClient()

  const [result, setResult] = useState<CheckUpdateResult | null>(null)
  const [agentRolloutId, setAgentRolloutId] = useState<number | null>(null)
  const [selectedServerIds, setSelectedServerIds] = useState<number[]>([])
  const [showServerPicker, setShowServerPicker] = useState(false)

  const serverAgents = result?.server_agents || []

  // When result is received, pre-select only servers that need update
  useEffect(() => {
    if (result) {
      setSelectedServerIds(
        (result.server_agents || []).filter(s => s.needs_update).map(s => s.id)
      )
    }
  }, [result?.agent_newer])

  const toggleServer = (id: number) => {
    setSelectedServerIds(prev =>
      prev.includes(id) ? prev.filter(s => s !== id) : [...prev, id]
    )
  }

  // Poll rollout status
  const { data: rolloutData } = useRollout(agentRolloutId ?? 0)
  const rollout = rolloutData?.data

  // Auto-refresh server_agents when rollout finishes.
  // Without this, the "X server(s) need update" badge stays stale because
  // result.server_agents is set once by checkUpdate and never updated by
  // useRollout polling — UI shows old needs_update=true even when servers
  // already upgraded successfully.
  const prevRolloutStatusRef = useRef<string | undefined>(undefined)
  useEffect(() => {
    const cur = rollout?.status
    const prev = prevRolloutStatusRef.current
    prevRolloutStatusRef.current = cur
    if (!cur) return
    const wasActive = prev === 'running' || prev === 'pending'
    const isTerminal = cur === 'completed' || cur === 'failed'
    if (wasActive && isTerminal) {
      checkUpdate.mutate(undefined, { onSuccess: setResult })
    }
  }, [rollout?.status])

  // Panel restart polling
  const [panelRestarting, setPanelRestarting] = useState(false)
  const [panelDone, setPanelDone] = useState<'success' | 'timeout' | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    if (!panelRestarting) return
    let attempts = 0
    pollRef.current = setInterval(async () => {
      attempts++
      try {
        await queryClient.fetchQuery({
          queryKey: ['panel-update-ping'],
          queryFn: async () => (await api.get('/settings/update/check')).data,
          staleTime: 0,
        })
        clearInterval(pollRef.current!)
        setPanelRestarting(false)
        setPanelDone('success')
        checkUpdate.mutate(undefined, { onSuccess: setResult })
      } catch {
        if (attempts >= 24) {
          clearInterval(pollRef.current!)
          setPanelRestarting(false)
          setPanelDone('timeout')
        }
      }
    }, 5000)
    return () => { if (pollRef.current) clearInterval(pollRef.current) }
  }, [panelRestarting])

  const handleCheck = () => {
    checkUpdate.mutate(undefined, { onSuccess: setResult })
  }

  const handleUpdatePanel = async () => {
    if (!result?.latest) return
    if (!confirm(`Update panel to ${result.latest.version}? It will restart for ~15–30s.`)) return
    setPanelDone(null)
    try {
      await applyPanel.mutateAsync({
        version: result.latest.version,
        download_url: result.latest.panel.download_url,
        checksum: result.latest.panel.checksum,
        web_url: result.latest.web?.download_url,
        web_checksum: result.latest.web?.checksum,
      })
      setPanelRestarting(true)
    } catch (e: any) {
      alert(e?.response?.data?.error || e.message)
    }
  }

  const handleUpdateAgents = async () => {
    if (!result?.latest) return
    const allNeedUpdate = serverAgents.filter(s => s.needs_update)
    const targetLabel = selectedServerIds.length === allNeedUpdate.length
      ? `all ${selectedServerIds.length} outdated server(s)` : `${selectedServerIds.length} selected server(s)`
    if (!confirm(`Update agents on ${targetLabel} to ${result.latest.version}?`)) return
    try {
      const res = await applyAgents.mutateAsync({
        version: result.latest.version,
        download_url: result.latest.agent.download_url,
        checksum: result.latest.agent.checksum,
        server_ids: selectedServerIds,
      })
      if (res?.rollout_id) setAgentRolloutId(res.rollout_id)
      setShowServerPicker(false)
    } catch (e: any) {
      alert(e?.response?.data?.error || e.message)
    }
  }

  const agentRunning = rollout?.status === 'running' || rollout?.status === 'pending'

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-5">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
          <RefreshCw size={20} className="text-blue-500" /> Updates
        </h2>
        <button
          onClick={handleCheck}
          disabled={checkUpdate.isPending}
          className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white text-sm font-semibold rounded-xl hover:bg-blue-700 disabled:opacity-50"
        >
          {checkUpdate.isPending ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
          Check for Updates
        </button>
      </div>

      {/* Last update result */}
      {lastResult && !panelRestarting && !panelDone && (
        <div className={`flex items-start gap-2 p-3 rounded-lg mb-4 text-sm border ${
          lastResult.status === 'completed' ? 'bg-green-50 border-green-100' : 'bg-red-50 border-red-100'
        }`}>
          {lastResult.status === 'completed'
            ? <CheckCircle size={15} className="text-green-500 mt-0.5 shrink-0" />
            : <AlertTriangle size={15} className="text-red-500 mt-0.5 shrink-0" />}
          <p className={lastResult.status === 'completed' ? 'text-green-800' : 'text-red-800'}>
            Last update: {lastResult.status === 'completed'
              ? `Panel upgraded to ${lastResult.new_version}`
              : `${lastResult.status} — ${lastResult.error || 'rolled back'}`}
            {lastResult.timestamp && <span className="text-gray-400 text-xs ml-2">{new Date(lastResult.timestamp).toLocaleString()}</span>}
          </p>
        </div>
      )}

      {/* Panel restart banner */}
      {panelRestarting && (
        <div className="flex items-center gap-3 p-4 bg-blue-50 border border-blue-200 rounded-xl mb-4">
          <Loader2 size={18} className="animate-spin text-blue-500 shrink-0" />
          <div>
            <p className="text-sm font-medium text-blue-800">Panel is restarting…</p>
            <p className="text-xs text-blue-600">Usually takes 15–30 seconds.</p>
          </div>
        </div>
      )}
      {panelDone === 'success' && (
        <div className="flex items-center gap-2 p-3 bg-green-50 border border-green-200 rounded-xl mb-4 text-sm">
          <CheckCircle size={16} className="text-green-500" />
          <span className="text-green-800 font-medium">Panel updated successfully.</span>
        </div>
      )}
      {panelDone === 'timeout' && (
        <div className="flex items-center gap-2 p-3 bg-red-50 border border-red-200 rounded-xl mb-4 text-sm">
          <XCircle size={16} className="text-red-500" />
          <span className="text-red-800">Panel did not come back within 2 minutes. Check server logs.</span>
        </div>
      )}

      {/* Check result */}
      {result ? (
        <div className="space-y-3">
          {/* Panel */}
          <div className={`flex items-center justify-between p-4 rounded-xl border ${
            result.panel_newer ? 'bg-amber-50 border-amber-200' : 'bg-gray-50 border-gray-100'
          }`}>
            <div>
              <p className="text-sm font-semibold text-gray-800">Panel (API)</p>
              <p className="text-xs text-gray-500 mt-0.5">
                Current: <span className="font-mono">{result.current_panel || 'unknown'}</span>
                {result.panel_newer && <span className="ml-2 text-amber-700">→ Latest: <span className="font-mono font-semibold">{result.latest?.version}</span></span>}
              </p>
            </div>
            {result.panel_newer ? (
              <button
                onClick={handleUpdatePanel}
                disabled={applyPanel.isPending || panelRestarting}
                className="flex items-center gap-1.5 px-3 py-1.5 bg-blue-600 text-white text-xs font-semibold rounded-lg hover:bg-blue-700 disabled:opacity-50"
              >
                {applyPanel.isPending || panelRestarting ? <Loader2 size={12} className="animate-spin" /> : <Download size={12} />}
                Update
              </button>
            ) : (
              <span className="text-xs text-green-600 font-medium flex items-center gap-1"><CheckCircle size={13} /> Up to date</span>
            )}
          </div>

          {/* Agent */}
          <div className={`p-4 rounded-xl border ${
            result.agent_newer ? 'bg-amber-50 border-amber-200' : 'bg-gray-50 border-gray-100'
          }`}>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-semibold text-gray-800">
                  Agent
                  {serverAgents.length > 0 && (
                    <span className="ml-2 text-xs font-normal text-gray-500">
                      {serverAgents.filter(s => !s.needs_update).length}/{serverAgents.length} up to date
                    </span>
                  )}
                </p>
                <p className="text-xs text-gray-500 mt-0.5">
                  {result.agent_newer
                    ? <span className="text-amber-700">{serverAgents.filter(s => s.needs_update).length} server(s) need update → <span className="font-mono font-semibold">{result.latest?.version}</span></span>
                    : <span>All servers up to date</span>
                  }
                </p>
                {/* Rollout progress */}
                {rollout && (
                  <p className={`text-xs mt-1 font-medium ${
                    rollout.status === 'completed' ? 'text-green-600' :
                    rollout.status === 'failed' ? 'text-red-600' : 'text-blue-600'
                  }`}>
                    {agentRunning && <Loader2 size={11} className="inline animate-spin mr-1" />}
                    {rollout.updated_count}/{rollout.total_servers} servers updated
                    {rollout.failed_count > 0 && <span className="text-red-500"> · {rollout.failed_count} failed</span>}
                    {' · '}{rollout.status}
                  </p>
                )}
              </div>
              {result.agent_newer ? (
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => setShowServerPicker(v => !v)}
                    className="flex items-center gap-1.5 px-3 py-1.5 bg-white border border-gray-200 text-gray-600 text-xs font-medium rounded-lg hover:bg-gray-50"
                  >
                    <Server size={12} />
                    {selectedServerIds.length} server{selectedServerIds.length !== 1 ? 's' : ''}
                  </button>
                  <button
                    onClick={handleUpdateAgents}
                    disabled={applyAgents.isPending || agentRunning || selectedServerIds.length === 0}
                    className="flex items-center gap-1.5 px-3 py-1.5 bg-blue-600 text-white text-xs font-semibold rounded-lg hover:bg-blue-700 disabled:opacity-50"
                  >
                    {applyAgents.isPending || agentRunning ? <Loader2 size={12} className="animate-spin" /> : <Download size={12} />}
                    Update
                  </button>
                </div>
              ) : (
                <span className="text-xs text-green-600 font-medium flex items-center gap-1"><CheckCircle size={13} /> Up to date</span>
              )}
            </div>

            {/* Server picker — shows all servers, pre-selects only outdated ones */}
            {showServerPicker && result.agent_newer && serverAgents.length > 0 && (
              <div className="mt-3 pt-3 border-t border-amber-200">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs font-medium text-gray-600">Select servers to update:</span>
                  <div className="flex gap-2">
                    <button onClick={() => setSelectedServerIds(serverAgents.filter(s => s.needs_update).map(s => s.id))} className="text-xs text-blue-600 hover:underline">Outdated only</button>
                    <button onClick={() => setSelectedServerIds(serverAgents.map(s => s.id))} className="text-xs text-gray-500 hover:underline">All</button>
                    <button onClick={() => setSelectedServerIds([])} className="text-xs text-gray-400 hover:underline">None</button>
                  </div>
                </div>
                <div className="space-y-1 max-h-48 overflow-y-auto">
                  {serverAgents.map(server => (
                    <label key={server.id} className="flex items-center gap-2.5 px-2 py-1.5 rounded-lg hover:bg-amber-100 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={selectedServerIds.includes(server.id)}
                        onChange={() => toggleServer(server.id)}
                        className="rounded border-gray-300 text-blue-600"
                      />
                      <Server size={12} className="text-gray-400 shrink-0" />
                      <span className="text-xs text-gray-700 font-medium">{server.name}</span>
                      <span className="text-xs text-gray-400 font-mono">{server.ip_address}</span>
                      <span className={`ml-auto text-xs font-mono font-medium ${server.needs_update ? 'text-amber-600' : 'text-green-600'}`}>
                        {server.agent_version || 'unknown'}
                        {!server.needs_update && ' ✓'}
                      </span>
                    </label>
                  ))}
                </div>
              </div>
            )}
          </div>

          {result.latest && (
            <div className="bg-gray-50 border border-gray-100 rounded-xl p-4">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-semibold text-gray-600 uppercase tracking-wide">
                  What's new in {result.latest.version}
                </span>
                <span className="text-xs text-gray-400">
                  {new Date(result.latest.released_at).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })}
                </span>
              </div>
              {result.latest.release_notes ? (
                <ul className="space-y-1">
                  {result.latest.release_notes.split('\\n').filter(Boolean).map((line, i) => (
                    <li key={i} className="text-xs text-gray-600 leading-relaxed">
                      {line.startsWith('•') ? line : `• ${line}`}
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-xs text-gray-400 italic">No release notes available.</p>
              )}
            </div>
          )}
        </div>
      ) : (
        <p className="text-sm text-gray-400 text-center py-6">
          Click "Check for Updates" to see if a new version is available.
        </p>
      )}
    </div>
  )
}
