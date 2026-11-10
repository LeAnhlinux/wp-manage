import { useState } from 'react'
import type { ReactNode } from 'react'
import { Bot, Loader2, AlertTriangle } from 'lucide-react'
import { BotRulesEditor } from './BotRulesEditor'
import { useBotCatalog, botErrorMessage } from '@/hooks/useBots'
import type { BotRule, BotRulesResponse } from '@/types'

interface Props {
  description: string
  /** Label and help text of the on/off switch for this scope. */
  switchLabel: string
  switchHelp: (on: boolean) => string
  /** Which field of the response the switch edits. */
  switchField: 'enabled' | 'inherit'
  data?: BotRulesResponse
  isLoading: boolean
  isSaving: boolean
  canManage: boolean
  onSave: (switchOn: boolean, rules: BotRule[]) => Promise<unknown>
  /** Extra notice rendered above the rules (e.g. server defaults are off). */
  notice?: (switchOn: boolean) => ReactNode
}

function toRule(r: BotRule): BotRule {
  return { bot_key: r.bot_key, user_agent: r.user_agent, action: r.action, rate_per_min: r.rate_per_min }
}

/** Shared card for the server-defaults tab and the per-site panel. */
export function BotControlCard({
  description, switchLabel, switchHelp, switchField, data, isLoading, isSaving, canManage, onSave, notice,
}: Props) {
  const { data: catalog, isLoading: catalogLoading } = useBotCatalog()

  // Local edits; null = showing what the server returned.
  const [draftRules, setDraftRules] = useState<BotRule[] | null>(null)
  const [draftSwitch, setDraftSwitch] = useState<boolean | null>(null)
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const rules = draftRules ?? (data?.rules ?? []).map(toRule)
  const switchOn = draftSwitch ?? (data ? data[switchField] : switchField === 'inherit')
  const dirty = draftRules !== null || draftSwitch !== null

  const inherited = switchField === 'inherit' && switchOn && data?.server_enabled
    ? (data.server_rules ?? [])
    : []

  const handleSave = async () => {
    setMsg(null)
    try {
      await onSave(switchOn, rules)
      setDraftRules(null)
      setDraftSwitch(null)
      setMsg({ type: 'success', text: 'Bot rules saved and applied.' })
    } catch (err) {
      setMsg({ type: 'error', text: botErrorMessage(err, 'Failed to save bot rules.') })
    }
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-2">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
          <Bot size={20} className="text-blue-600" /> Bot Control
        </h2>
        {(isLoading || catalogLoading) && <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-blue-600" />}
      </div>
      <p className="text-sm text-gray-500 mb-5">{description}</p>

      <div className={`flex items-center justify-between gap-4 p-4 rounded-lg border mb-5 ${
        switchOn ? 'border-blue-200 bg-blue-50' : 'border-gray-200 bg-gray-50'
      }`}>
        <div>
          <p className="text-sm font-medium text-gray-900">{switchLabel}</p>
          <p className="text-xs text-gray-500 mt-0.5">{switchHelp(switchOn)}</p>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={switchOn}
          aria-label={switchLabel}
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

      {notice?.(switchOn)}

      {catalog && data && (
        <BotRulesEditor
          catalog={catalog.bots}
          rateTiers={catalog.rate_tiers}
          rules={rules}
          onChange={setDraftRules}
          inherited={inherited}
          disabled={!canManage || isSaving}
        />
      )}

      <div className="flex items-start gap-2 text-xs text-gray-500 mt-5">
        <AlertTriangle size={14} className="text-amber-500 flex-shrink-0 mt-0.5" />
        <span>
          Bots are matched by User-Agent, which can be spoofed: a client pretending to be an allowed bot is treated
          as that bot. Denied bots get 403; rate-limited bots get 429 above the limit, counted per bot across all its IPs.
        </span>
      </div>

      {msg && (
        <p role="status" className={`text-sm mt-4 ${msg.type === 'success' ? 'text-green-600' : 'text-red-600'}`}>
          {msg.text}
        </p>
      )}

      {canManage && (
        <button
          type="button"
          onClick={handleSave}
          disabled={isSaving || !dirty || !data}
          className="mt-4 flex items-center gap-2 px-4 py-2.5 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
        >
          {isSaving ? <><Loader2 size={15} className="animate-spin" /> Applying on server…</> : <><Bot size={15} /> Save Bot Rules</>}
        </button>
      )}
    </div>
  )
}
