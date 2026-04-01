import { useState } from 'react'
import { Loader2, Lock, LockOpen, RefreshCw, Shield, Key, CheckCircle2, XCircle } from 'lucide-react'
import { useSSLStatus, useEnableSSL, useDisableSSL, useRenewSSL, useUploadCustomSSL, useForceHTTPS } from '@/hooks/useSSL'

export function SSLPanel({ siteId, domain, canManage }: { siteId: number; domain: string; canManage: boolean }) {
  const { data: status, isLoading, refetch: refetchStatus } = useSSLStatus(siteId)
  const enableSSL = useEnableSSL(siteId)
  const disableSSL = useDisableSSL(siteId)
  const renewSSL = useRenewSSL(siteId)
  const uploadSSL = useUploadCustomSSL(siteId)
  const forceHTTPS = useForceHTTPS(siteId)
  const [showUpload, setShowUpload] = useState(false)
  const [certForm, setCertForm] = useState({ cert_pem: '', key_pem: '', ca_pem: '' })
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const showMsg = (type: 'success' | 'error', text: string, ms = 6000) => {
    setMsg({ type, text })
    setTimeout(() => setMsg(null), ms)
  }

  const handleEnable = async () => {
    try {
      const res = await enableSSL.mutateAsync()
      showMsg('success', res.message || 'SSL certificate issued successfully! Site now serves HTTPS.')
      refetchStatus()
    } catch (err: any) {
      showMsg('error', err?.response?.data?.error || 'Failed to enable SSL. Check certbot logs.')
    }
  }

  const handleDisable = async () => {
    if (!confirm(`Remove SSL from ${domain}? The site will revert to HTTP only.`)) return
    try {
      const res = await disableSSL.mutateAsync()
      showMsg('success', res.message || 'SSL disabled. Site now serves HTTP only.')
      refetchStatus()
    } catch (err: any) {
      showMsg('error', err?.response?.data?.error || 'Failed to disable SSL.')
    }
  }

  const handleRenew = async () => {
    try {
      const res = await renewSSL.mutateAsync()
      showMsg('success', res.message || 'SSL certificate renewed successfully!')
      refetchStatus()
    } catch (err: any) {
      showMsg('error', err?.response?.data?.error || 'Failed to renew SSL.')
    }
  }

  const handleForceHTTPS = async (enable: boolean) => {
    try {
      await forceHTTPS.mutateAsync(enable)
      showMsg('success', enable ? 'Force HTTPS enabled — HTTP traffic will redirect to HTTPS.' : 'Force HTTPS disabled.')
      refetchStatus()
    } catch {
      showMsg('error', 'Failed to update Force HTTPS setting')
    }
  }

  const handleUpload = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await uploadSSL.mutateAsync(certForm)
      showMsg('success', 'Custom SSL certificate uploaded successfully')
      setShowUpload(false)
      setCertForm({ cert_pem: '', key_pem: '', ca_pem: '' })
    } catch {
      showMsg('error', 'Failed to upload SSL certificate')
    }
  }

  const anyPending = enableSSL.isPending || disableSSL.isPending || renewSSL.isPending

  if (isLoading) {
    return <div className="flex items-center justify-center py-12"><Loader2 size={24} className="animate-spin text-gray-400" /></div>
  }

  const isEnabled = status?.ssl_enabled ?? false
  const expiresAt = status?.ssl_expires_at

  // Calculate days until expiry
  let daysLeft = 0
  let expiryColor = 'text-green-600'
  if (expiresAt) {
    daysLeft = Math.ceil((new Date(expiresAt).getTime() - Date.now()) / (1000 * 60 * 60 * 24))
    if (daysLeft <= 7) expiryColor = 'text-red-600'
    else if (daysLeft <= 30) expiryColor = 'text-yellow-600'
  }

  return (
    <div className="space-y-6">
      {msg && (
        <div className={`p-3 rounded-lg text-sm ${msg.type === 'success' ? 'bg-green-50 text-green-700 border border-green-200' : 'bg-red-50 text-red-700 border border-red-200'}`}>
          {msg.text}
        </div>
      )}

      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Lock size={20} className="text-green-500" /> SSL Certificate
          </h2>
          {canManage && (
            <div className="flex gap-2 flex-wrap">
              <button onClick={() => setShowUpload(!showUpload)} disabled={anyPending}
                className="flex items-center gap-1.5 px-3 py-2 text-sm border border-gray-200 rounded-lg hover:bg-gray-50 disabled:opacity-50">
                <Key size={14} /> Custom SSL
              </button>
              {!isEnabled ? (
                <button onClick={handleEnable} disabled={anyPending}
                  className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-green-600 rounded-lg hover:bg-green-700 disabled:opacity-50">
                  {enableSSL.isPending ? <Loader2 size={14} className="animate-spin" /> : <Shield size={14} />}
                  {enableSSL.isPending ? 'Issuing SSL…' : 'Enable SSL'}
                </button>
              ) : (
                <>
                  <button onClick={handleRenew} disabled={anyPending}
                    className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
                    {renewSSL.isPending ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
                    {renewSSL.isPending ? 'Renewing…' : 'Renew SSL'}
                  </button>
                  <button onClick={handleDisable} disabled={anyPending}
                    className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-red-600 rounded-lg hover:bg-red-700 disabled:opacity-50">
                    {disableSSL.isPending ? <Loader2 size={14} className="animate-spin" /> : <LockOpen size={14} />}
                    {disableSSL.isPending ? 'Disabling…' : 'Disable SSL'}
                  </button>
                </>
              )}
            </div>
          )}
        </div>

        {anyPending && (
          <div className="mb-4 p-3 bg-blue-50 border border-blue-200 rounded-lg text-sm text-blue-700 flex items-center gap-2">
            <Loader2 size={14} className="animate-spin" />
            {enableSSL.isPending ? 'Issuing SSL certificate via Let\'s Encrypt… this may take up to 60 seconds.' :
             disableSSL.isPending ? 'Removing SSL certificate and updating nginx config…' :
             'Renewing SSL certificate… this may take up to 60 seconds.'}
          </div>
        )}

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
          <div className={`p-4 rounded-lg ${isEnabled ? 'bg-green-50 border border-green-200' : 'bg-red-50 border border-red-200'}`}>
            <p className="text-xs text-gray-500 uppercase font-medium mb-1">Status</p>
            <div className="flex items-center gap-2">
              {isEnabled ? <CheckCircle2 size={20} className="text-green-500" /> : <XCircle size={20} className="text-red-500" />}
              <p className={`text-lg font-semibold ${isEnabled ? 'text-green-600' : 'text-red-600'}`}>
                {isEnabled ? 'Active' : 'Disabled'}
              </p>
            </div>
          </div>
          <div className="p-4 bg-gray-50 rounded-lg">
            <p className="text-xs text-gray-500 uppercase font-medium mb-1">Domain</p>
            <p className="text-lg font-semibold text-gray-900">{domain}</p>
          </div>
          <div className="p-4 bg-gray-50 rounded-lg">
            <p className="text-xs text-gray-500 uppercase font-medium mb-1">Expires</p>
            {expiresAt ? (
              <div>
                <p className={`text-lg font-semibold ${expiryColor}`}>{daysLeft} days left</p>
                <p className="text-xs text-gray-500">{new Date(expiresAt).toLocaleDateString()}</p>
              </div>
            ) : (
              <p className="text-lg font-semibold text-gray-400">N/A</p>
            )}
          </div>
        </div>

        {/* Force HTTPS toggle — only shown when SSL is active */}
        {isEnabled && (() => {
          const isForceHTTPS = status?.force_https ?? false
          return (
            <div className="flex items-center justify-between p-4 bg-gray-50 rounded-lg border border-gray-200">
              <div>
                <p className="text-sm font-medium text-gray-900">Force HTTPS</p>
                <p className="text-xs text-gray-500 mt-0.5">Redirect all HTTP traffic to HTTPS via Nginx</p>
              </div>
              {canManage ? (
                <div className="flex items-center gap-3">
                  {forceHTTPS.isPending ? (
                    <Loader2 size={16} className="animate-spin text-gray-400" />
                  ) : (
                    <span className={`text-xs font-medium ${isForceHTTPS ? 'text-green-600' : 'text-gray-400'}`}>
                      {isForceHTTPS ? 'Enabled' : 'Disabled'}
                    </span>
                  )}
                  <button
                    onClick={() => handleForceHTTPS(!isForceHTTPS)}
                    disabled={forceHTTPS.isPending}
                    className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors disabled:opacity-50 focus:outline-none ${
                      isForceHTTPS ? 'bg-green-500' : 'bg-gray-300'
                    }`}
                    role="switch"
                    aria-checked={isForceHTTPS}>
                    <span
                      className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
                        isForceHTTPS ? 'translate-x-6' : 'translate-x-1'
                      }`}
                    />
                  </button>
                </div>
              ) : (
                <div className="flex items-center gap-2">
                  <span className={`text-xs font-medium ${isForceHTTPS ? 'text-green-600' : 'text-gray-400'}`}>
                    {isForceHTTPS ? 'Enabled' : 'Disabled'}
                  </span>
                  <div className={`relative inline-flex h-6 w-11 items-center rounded-full ${isForceHTTPS ? 'bg-green-500' : 'bg-gray-300'} opacity-50`}>
                    <span className={`inline-block h-4 w-4 transform rounded-full bg-white shadow ${isForceHTTPS ? 'translate-x-6' : 'translate-x-1'}`} />
                  </div>
                </div>
              )}
            </div>
          )
        })()}
      </div>

      {/* Custom SSL Upload Form */}
      {showUpload && (
        <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
          <h3 className="text-md font-semibold text-gray-900 mb-4">Upload Custom SSL Certificate</h3>
          <form onSubmit={handleUpload} className="space-y-4">
            <div>
              <label className="block text-sm text-gray-600 mb-1">Certificate (PEM)</label>
              <textarea value={certForm.cert_pem} onChange={e => setCertForm({...certForm, cert_pem: e.target.value})}
                placeholder="-----BEGIN CERTIFICATE-----&#10;..."
                className="w-full h-28 px-3 py-2 border rounded-lg text-sm font-mono resize-none" required />
            </div>
            <div>
              <label className="block text-sm text-gray-600 mb-1">Private Key (PEM)</label>
              <textarea value={certForm.key_pem} onChange={e => setCertForm({...certForm, key_pem: e.target.value})}
                placeholder="-----BEGIN PRIVATE KEY-----&#10;..."
                className="w-full h-28 px-3 py-2 border rounded-lg text-sm font-mono resize-none" required />
            </div>
            <div>
              <label className="block text-sm text-gray-600 mb-1">CA Bundle (optional)</label>
              <textarea value={certForm.ca_pem} onChange={e => setCertForm({...certForm, ca_pem: e.target.value})}
                placeholder="-----BEGIN CERTIFICATE-----&#10;..."
                className="w-full h-28 px-3 py-2 border rounded-lg text-sm font-mono resize-none" />
            </div>
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => setShowUpload(false)} className="px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 rounded-lg">Cancel</button>
              <button type="submit" disabled={uploadSSL.isPending}
                className="px-4 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {uploadSSL.isPending ? 'Uploading...' : 'Upload Certificate'}
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  )
}
