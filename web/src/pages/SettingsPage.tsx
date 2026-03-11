import { useState } from 'react'
import { Key, Plus, Trash2, Copy, Check, ShieldCheck, ShieldOff, Bell, Save, Settings, Loader2 } from 'lucide-react'
import { useAPIKeys, useCreateAPIKey, useDeleteAPIKey } from '@/hooks/useAPIKeys'
import { useEnable2FA, useVerify2FA, useDisable2FA } from '@/hooks/useTwoFactor'
import { useNotificationConfig, useUpdateNotificationConfig } from '@/hooks/useAlerts'
import { usePanelUpdateStatus, usePanelReleases } from '@/hooks/usePanelUpdate'
import { useAuth } from '@/stores/auth'

export default function SettingsPage() {
  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Settings</h1>
      <div className="space-y-6">
        <TwoFactorSection />
        <NotificationSection />
        <APIKeysSection />
        <PanelUpdateSection />
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
              <img
                src={`https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(setupData.qr_code_uri)}`}
                alt="QR Code"
                className="w-48 h-48 border rounded-lg bg-white p-2"
              />
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
    slack_webhook: '',
    smtp_host: '', smtp_port: 587, smtp_user: '', smtp_pass: '', smtp_from: '',
  })
  const [initialized, setInitialized] = useState(false)

  if (config && !initialized) {
    setForm({
      telegram_token: config.telegram_token || '',
      telegram_chat: config.telegram_chat || '',
      slack_webhook: config.slack_webhook || '',
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

        {/* Slack */}
        <div>
          <h3 className="text-sm font-medium text-gray-700 mb-2">Slack</h3>
          <input value={form.slack_webhook} onChange={e => setForm({ ...form, slack_webhook: e.target.value })}
            className="w-full px-3 py-2 border rounded-lg text-sm" placeholder="Webhook URL" />
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

function PanelUpdateSection() {
  const { data: status, isLoading } = usePanelUpdateStatus()
  const { data: releases = [] } = usePanelReleases()

  if (isLoading) return <div className="text-center py-4"><Loader2 size={20} className="animate-spin mx-auto" /></div>

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <h2 className="text-lg font-semibold text-gray-900 mb-4 flex items-center gap-2">
        <Settings size={20} className="text-blue-500" /> Panel Version
      </h2>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
        <div className="p-4 bg-blue-50 rounded-lg">
          <p className="text-xs text-gray-500 uppercase font-medium mb-1">Current Version</p>
          <p className="text-lg font-semibold text-blue-600">{status?.current_version || 'Unknown'}</p>
        </div>
        <div className={`p-4 rounded-lg ${status?.update_available ? 'bg-green-50' : 'bg-gray-50'}`}>
          <p className="text-xs text-gray-500 uppercase font-medium mb-1">Update Status</p>
          <p className={`text-lg font-semibold ${status?.update_available ? 'text-green-600' : 'text-gray-600'}`}>
            {status?.update_available ? 'Update Available!' : 'Up to date'}
          </p>
        </div>
        {status?.latest && (
          <div className="p-4 bg-green-50 rounded-lg">
            <p className="text-xs text-gray-500 uppercase font-medium mb-1">Latest Version</p>
            <p className="text-lg font-semibold text-green-600">{status.latest.version}</p>
          </div>
        )}
      </div>

      {/* Recent Releases */}
      {releases.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold text-gray-700 mb-2">Recent Releases</h3>
          <div className="space-y-2">
            {releases.slice(0, 5).map(r => (
              <div key={r.id} className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                <div>
                  <p className="text-sm font-medium">{r.version}</p>
                  <p className="text-xs text-gray-500">{r.release_notes || 'No release notes'}</p>
                </div>
                <p className="text-xs text-gray-400">{new Date(r.created_at).toLocaleDateString()}</p>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
