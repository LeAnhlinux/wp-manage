import { useMemo, useState } from 'react'
import { Globe, Loader2, AlertTriangle, RefreshCw, X } from 'lucide-react'
import { useGeoCatalog, useRefreshGeoProviders, geoErrorMessage } from '@/hooks/useGeo'
import type { GeoAction, GeoMode, GeoProviderStatus, GeoRuleSet, GeoRulesResponse } from '@/types'

const MODES: { id: GeoMode; label: string; help: string }[] = [
  { id: 'off', label: 'Off', help: 'No country restriction.' },
  { id: 'blacklist', label: 'Block selected', help: 'Visitors from the selected countries get 403.' },
  { id: 'whitelist', label: 'Allow only selected', help: 'Only visitors from the selected countries can reach the site.' },
]

let regionNames: Intl.DisplayNames | null = null
try {
  regionNames = new Intl.DisplayNames(['en'], { type: 'region' })
} catch {
  regionNames = null
}

function countryName(code: string): string {
  try {
    return regionNames?.of(code) || code
  } catch {
    return code
  }
}

/** One-line description of a rule set, for the "inherited" summary. */
function summarize(rule: GeoRuleSet, providers: GeoProviderStatus[]): string {
  const parts: string[] = []
  if (rule.mode === 'blacklist') parts.push(`Blocking ${rule.countries.length} ${rule.countries.length === 1 ? 'country' : 'countries'} (${rule.countries.join(', ')})`)
  if (rule.mode === 'whitelist') parts.push(`Allowing only ${rule.countries.join(', ')}`)
  const name = (key: string) => providers.find(p => p.key === key)?.name || key
  const allowed = Object.keys(rule.providers).filter(k => rule.providers[k] === 'allow').map(name)
  const denied = Object.keys(rule.providers).filter(k => rule.providers[k] === 'deny').map(name)
  if (allowed.length) parts.push(`always allowing ${allowed.join(', ')}`)
  if (denied.length) parts.push(`blocking ${denied.join(', ')}`)
  return parts.length ? parts.join(' · ') : 'No restrictions'
}

interface Props {
  scope: 'server' | 'site'
  data?: GeoRulesResponse
  isLoading: boolean
  isSaving: boolean
  canManage: boolean
  onSave: (switchOn: boolean, rule: GeoRuleSet) => Promise<unknown>
}

/** Shared card for the server-defaults tab and the per-site panel. */
export function GeoAccessCard({ scope, data, isLoading, isSaving, canManage, onSave }: Props) {
  const { data: catalog, isLoading: catalogLoading } = useGeoCatalog()
  const refreshProviders = useRefreshGeoProviders()

  // Local edits; null = showing what the server returned.
  const [draftSwitch, setDraftSwitch] = useState<boolean | null>(null)
  const [draftMode, setDraftMode] = useState<GeoMode | null>(null)
  const [draftCountries, setDraftCountries] = useState<string[] | null>(null)
  const [draftProviders, setDraftProviders] = useState<Record<string, GeoAction> | null>(null)
  const [search, setSearch] = useState('')
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const isSite = scope === 'site'
  const switchOn = draftSwitch ?? (data ? (isSite ? data.inherit : data.enabled) : isSite)
  const mode = draftMode ?? data?.mode ?? 'off'
  const countries = draftCountries ?? data?.countries ?? []
  const providers = draftProviders ?? data?.providers ?? {}
  const dirty = draftSwitch !== null || draftMode !== null || draftCountries !== null || draftProviders !== null

  // A site that inherits has nothing of its own to edit.
  const editing = isSite ? !switchOn : true
  const disabled = !canManage || isSaving
  const needsCountry = editing && mode !== 'off' && countries.length === 0

  const allCountries = useMemo(
    () => (catalog?.countries ?? []).map(code => ({ code, name: countryName(code) })).sort((a, b) => a.name.localeCompare(b.name)),
    [catalog],
  )
  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return allCountries
    return allCountries.filter(c => c.name.toLowerCase().includes(q) || c.code.toLowerCase() === q)
  }, [allCountries, search])

  const toggleCountry = (code: string) => {
    setDraftCountries(countries.includes(code) ? countries.filter(c => c !== code) : [...countries, code].sort())
  }

  const setProvider = (key: string, action: GeoAction | null) => {
    const next = { ...providers }
    if (action) next[key] = action
    else delete next[key]
    setDraftProviders(next)
  }

  const handleSave = async () => {
    setMsg(null)
    try {
      await onSave(switchOn, { mode, countries, providers })
      setDraftSwitch(null)
      setDraftMode(null)
      setDraftCountries(null)
      setDraftProviders(null)
      setMsg({ type: 'success', text: 'Country access saved and applied.' })
    } catch (err) {
      setMsg({ type: 'error', text: geoErrorMessage(err, 'Failed to save country access.') })
    }
  }

  const handleRefresh = async () => {
    setMsg(null)
    try {
      await refreshProviders.mutateAsync()
      setMsg({ type: 'success', text: 'Provider IP ranges refreshed.' })
    } catch (err) {
      setMsg({ type: 'error', text: geoErrorMessage(err, 'Failed to refresh provider IP ranges.') })
    }
  }

  const choiceClass = (active: boolean, tone: 'neutral' | 'allow' | 'deny') => {
    if (!active) return 'text-gray-600 hover:bg-gray-50'
    if (tone === 'deny') return 'bg-red-600 text-white'
    if (tone === 'allow') return 'bg-green-600 text-white'
    return 'bg-gray-700 text-white'
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-2">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
          <Globe size={20} className="text-blue-600" /> Country Access
        </h2>
        {(isLoading || catalogLoading) && <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-blue-600" />}
      </div>
      <p className="text-sm text-gray-500 mb-5">
        {isSite
          ? 'Block or allow visitors by country, and always allow or block large providers such as Google, Cloudflare or Facebook.'
          : 'Default country and provider rules for every site on this server. A site can switch to its own rules in its Security tab.'}
      </p>

      <div className={`flex items-center justify-between gap-4 p-4 rounded-lg border mb-5 ${
        switchOn ? 'border-blue-200 bg-blue-50' : 'border-gray-200 bg-gray-50'
      }`}>
        <div>
          <p className="text-sm font-medium text-gray-900">{isSite ? 'Use server defaults' : 'Enable server defaults'}</p>
          <p className="text-xs text-gray-500 mt-0.5">
            {isSite
              ? (switchOn ? 'This site follows the rules set on its server.' : 'This site uses its own rules below.')
              : (switchOn ? 'These rules apply to every site that uses server defaults.' : 'Rules are saved but not applied to any site.')}
          </p>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={switchOn}
          aria-label={isSite ? 'Use server defaults' : 'Enable server defaults'}
          onClick={() => setDraftSwitch(!switchOn)}
          disabled={!canManage}
          className={`relative inline-flex h-6 w-11 flex-shrink-0 items-center rounded-full transition-colors focus:outline-none ${
            switchOn ? 'bg-blue-600' : 'bg-gray-300'
          } disabled:opacity-50`}
        >
          <span className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
            switchOn ? 'translate-x-6' : 'translate-x-1'
          }`} />
        </button>
      </div>

      {isSite && switchOn && data && catalog && (
        <p className="text-sm text-gray-700 mb-2" data-testid="geo-inherited">
          <span className="font-medium">Server defaults: </span>
          {data.server_enabled && data.server ? summarize(data.server, catalog.providers) : 'turned off — no country rules apply to this site.'}
        </p>
      )}

      {editing && data && catalog && (
        <div className="space-y-5">
          <section>
            <h3 className="text-sm font-semibold text-gray-900 mb-2">Country rule</h3>
            <div role="group" aria-label="Country rule mode" className="inline-flex flex-wrap rounded-lg border border-gray-200 overflow-hidden">
              {MODES.map(m => (
                <button
                  key={m.id}
                  type="button"
                  disabled={disabled}
                  aria-pressed={mode === m.id}
                  onClick={() => setDraftMode(m.id)}
                  className={`px-3 py-2 text-sm font-medium border-l border-gray-200 first:border-l-0 transition-colors disabled:opacity-50 ${
                    choiceClass(mode === m.id, m.id === 'blacklist' ? 'deny' : m.id === 'whitelist' ? 'allow' : 'neutral')
                  }`}
                >
                  {m.label}
                </button>
              ))}
            </div>
            <p className="text-xs text-gray-500 mt-1.5">{MODES.find(m => m.id === mode)?.help}</p>
            {mode === 'whitelist' && (
              <p className="flex items-start gap-1.5 text-xs text-amber-700 mt-1.5">
                <AlertTriangle size={14} className="flex-shrink-0 mt-0.5" />
                Every other country is blocked. Make sure the countries you and your customers browse from are selected.
              </p>
            )}
          </section>

          {mode !== 'off' && (
            <section>
              <h3 className="text-sm font-semibold text-gray-900 mb-2">
                Countries <span className="font-normal text-gray-500">({countries.length} selected)</span>
              </h3>
              {countries.length > 0 && (
                <div className="flex flex-wrap gap-1.5 mb-2">
                  {countries.map(code => (
                    <span key={code} className="inline-flex items-center gap-1 text-xs px-2 py-1 rounded-full bg-blue-50 text-blue-700 border border-blue-200">
                      {countryName(code)}
                      <button type="button" disabled={disabled} onClick={() => toggleCountry(code)} aria-label={`Remove ${countryName(code)}`} className="hover:text-blue-900 disabled:opacity-50">
                        <X size={12} />
                      </button>
                    </span>
                  ))}
                </div>
              )}
              <input
                type="text"
                value={search}
                onChange={e => setSearch(e.target.value)}
                placeholder="Search countries…"
                aria-label="Search countries"
                className="w-full text-sm px-3 py-2 border border-gray-200 rounded-lg bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none mb-2"
              />
              <div className="max-h-56 overflow-y-auto border border-gray-200 rounded-lg p-2 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-x-3">
                {filtered.map(c => (
                  <label key={c.code} className="flex items-center gap-2 py-1 px-1 text-sm text-gray-800 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={countries.includes(c.code)}
                      disabled={disabled}
                      onChange={() => toggleCountry(c.code)}
                      className="rounded border-gray-300"
                    />
                    <span className="truncate">{c.name}</span>
                    <span className="text-xs text-gray-400 font-mono">{c.code}</span>
                  </label>
                ))}
                {filtered.length === 0 && <p className="text-sm text-gray-500 p-2">No country matches “{search}”.</p>}
              </div>
              {needsCountry && <p className="text-xs text-red-600 mt-1.5">Select at least one country.</p>}
            </section>
          )}

          <section>
            <div className="flex items-center justify-between gap-2">
              <h3 className="text-sm font-semibold text-gray-900">Providers</h3>
              {canManage && (
                <button
                  type="button"
                  onClick={handleRefresh}
                  disabled={refreshProviders.isPending}
                  className="flex items-center gap-1.5 text-xs text-gray-600 hover:text-gray-900 disabled:opacity-50"
                >
                  <RefreshCw size={13} className={refreshProviders.isPending ? 'animate-spin' : ''} /> Refresh IP ranges
                </button>
              )}
            </div>
            <p className="text-xs text-gray-500 mb-2">
              A provider rule wins over the country rule: “Allow” lets the provider through even from a blocked country.
            </p>
            <div className="border border-gray-200 rounded-lg divide-y divide-gray-100">
              {catalog.providers.map(p => {
                const current = providers[p.key]
                return (
                  <div key={p.key} className="flex flex-wrap items-center gap-x-3 gap-y-2 py-2.5 px-3">
                    <div className="min-w-0 flex-1 basis-48">
                      <p className="text-sm font-medium text-gray-900">{p.name}</p>
                      <p className="text-xs text-gray-500">{p.description}</p>
                      <p className="text-xs text-gray-400">
                        {p.range_count > 0
                          ? `${p.range_count.toLocaleString()} IP ranges${p.fetched_at ? ` · updated ${new Date(p.fetched_at).toLocaleDateString()}` : ''}`
                          : 'No IP ranges downloaded yet'}
                        {p.last_error ? ' · last refresh failed' : ''}
                      </p>
                    </div>
                    <div role="group" aria-label={`Rule for ${p.name}`} className="inline-flex rounded-lg border border-gray-200 overflow-hidden">
                      {([['Default', null, 'neutral'], ['Allow', 'allow', 'allow'], ['Deny', 'deny', 'deny']] as const).map(([label, action, tone]) => (
                        <button
                          key={label}
                          type="button"
                          disabled={disabled}
                          aria-pressed={(current ?? null) === action}
                          onClick={() => setProvider(p.key, action)}
                          className={`px-2.5 py-1.5 text-xs font-medium border-l border-gray-200 first:border-l-0 transition-colors disabled:opacity-50 ${
                            choiceClass((current ?? null) === action, tone)
                          }`}
                        >
                          {label}
                        </button>
                      ))}
                    </div>
                  </div>
                )
              })}
            </div>
          </section>
        </div>
      )}

      <p className="text-xs text-gray-500 mt-5">
        Always allowed: Let’s Encrypt validation, this server’s own requests (WordPress cron), and visitors whose country
        cannot be determined. Country data is approximate and VPNs bypass it.{' '}
        <a href="https://db-ip.com" target="_blank" rel="noreferrer" className="underline">IP Geolocation by DB-IP</a>.
      </p>

      {msg && (
        <p role="status" className={`text-sm mt-4 ${msg.type === 'success' ? 'text-green-600' : 'text-red-600'}`}>
          {msg.text}
        </p>
      )}

      {canManage && (
        <button
          type="button"
          onClick={handleSave}
          disabled={isSaving || !dirty || !data || needsCountry}
          className="mt-4 flex items-center gap-2 px-4 py-2.5 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
        >
          {isSaving ? <><Loader2 size={15} className="animate-spin" /> Applying on server…</> : <><Globe size={15} /> Save Country Access</>}
        </button>
      )}
    </div>
  )
}
