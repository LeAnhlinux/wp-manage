import { useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import type { BotAction, BotCatalogEntry, BotCategory, BotRule } from '@/types'

const CUSTOM_PREFIX = 'custom:'
// Must match the API / agent validation (models.ValidBotPattern).
const UA_PATTERN = /^[A-Za-z0-9 ._/-]{2,100}$/

const CATEGORIES: { id: BotCategory; label: string; hint: string }[] = [
  { id: 'search', label: 'Search engines', hint: 'Blocking these removes the site from search results.' },
  { id: 'ai', label: 'AI crawlers', hint: 'Crawlers that collect content for AI training and AI search.' },
  { id: 'seo', label: 'SEO tools', hint: 'Backlink and keyword crawlers. Often heavy, rarely useful to the site.' },
  { id: 'scraper', label: 'Scrapers & HTTP clients', hint: 'Generic clients. Blocking them can break integrations that call the site.' },
]

const ACTION_LABEL: Record<BotAction, string> = {
  allow: 'Allow',
  rate_limit: 'Rate limit',
  deny: 'Deny',
}

type Choice = BotAction | 'default'
const CHOICES: Choice[] = ['default', 'allow', 'rate_limit', 'deny']

function describe(rule: BotRule): string {
  return rule.action === 'rate_limit' ? `Rate limit ${rule.rate_per_min}/min` : ACTION_LABEL[rule.action]
}

interface RowProps {
  name: string
  token: string
  rule?: BotRule
  inherited?: BotRule
  rateTiers: number[]
  disabled?: boolean
  onSet: (action: Choice, rate: number) => void
  onRemove?: () => void
}

function BotRow({ name, token, rule, inherited, rateTiers, disabled, onSet, onRemove }: RowProps) {
  const current: Choice = rule?.action ?? 'default'
  const defaultRate = rateTiers.includes(30) ? 30 : rateTiers[0]

  const activeClass = (c: Choice) => {
    if (c !== current) return 'text-gray-600 hover:bg-gray-50'
    if (c === 'deny') return 'bg-red-600 text-white'
    if (c === 'rate_limit') return 'bg-amber-500 text-white'
    if (c === 'allow') return 'bg-green-600 text-white'
    return 'bg-gray-700 text-white'
  }

  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2 py-2.5 px-3">
      <div className="min-w-0 flex-1 basis-40">
        <p className="text-sm font-medium text-gray-900 truncate">{name}</p>
        {token !== name && <p className="text-xs text-gray-500 font-mono truncate">{token}</p>}
      </div>

      {current === 'default' && inherited && (
        <span className="text-xs px-2 py-0.5 rounded-full bg-gray-100 text-gray-600 whitespace-nowrap">
          Inherited: {describe(inherited)}
        </span>
      )}

      {current === 'rate_limit' && (
        <select
          aria-label={`Rate limit for ${name}`}
          value={rule!.rate_per_min}
          disabled={disabled}
          onChange={e => onSet('rate_limit', Number(e.target.value))}
          className="text-xs px-2 py-1.5 border border-gray-200 rounded-lg bg-white disabled:opacity-50"
        >
          {rateTiers.map(t => <option key={t} value={t}>{t} / min</option>)}
        </select>
      )}

      <div role="group" aria-label={`Action for ${name}`} className="inline-flex rounded-lg border border-gray-200 overflow-hidden">
        {CHOICES.map(c => (
          <button
            key={c}
            type="button"
            disabled={disabled}
            aria-pressed={c === current}
            onClick={() => onSet(c, rule?.rate_per_min || defaultRate)}
            className={`px-2.5 py-1.5 text-xs font-medium border-l border-gray-200 first:border-l-0 transition-colors disabled:opacity-50 ${activeClass(c)}`}
          >
            {c === 'default' ? 'Default' : ACTION_LABEL[c]}
          </button>
        ))}
      </div>

      {onRemove && (
        <button
          type="button"
          onClick={onRemove}
          disabled={disabled}
          aria-label={`Remove ${name}`}
          className="p-1.5 text-gray-400 hover:text-red-600 disabled:opacity-50"
        >
          <Trash2 size={15} />
        </button>
      )}
    </div>
  )
}

interface Props {
  catalog: BotCatalogEntry[]
  rateTiers: number[]
  rules: BotRule[]
  onChange: (rules: BotRule[]) => void
  /** Rules from the server defaults that apply when a bot is left on "Default". */
  inherited?: BotRule[]
  disabled?: boolean
}

export function BotRulesEditor({ catalog, rateTiers, rules, onChange, inherited = [], disabled }: Props) {
  const [customUA, setCustomUA] = useState('')
  const [customError, setCustomError] = useState<string | null>(null)

  const byKey = new Map(rules.map(r => [r.bot_key, r]))
  const inheritedByKey = new Map(inherited.map(r => [r.bot_key, r]))

  const setRule = (botKey: string, userAgent: string, action: Choice, rate: number) => {
    const rest = rules.filter(r => r.bot_key !== botKey)
    if (action === 'default') {
      onChange(rest)
      return
    }
    onChange([...rest, {
      bot_key: botKey,
      user_agent: userAgent,
      action,
      rate_per_min: action === 'rate_limit' ? rate : 0,
    }])
  }

  // Custom rows: this scope's own custom rules plus inherited ones it can override.
  const customKeys = Array.from(new Set([
    ...rules.filter(r => r.bot_key.startsWith(CUSTOM_PREFIX)).map(r => r.bot_key),
    ...inherited.filter(r => r.bot_key.startsWith(CUSTOM_PREFIX)).map(r => r.bot_key),
  ])).sort()

  const addCustom = () => {
    const ua = customUA.trim()
    if (!UA_PATTERN.test(ua)) {
      setCustomError('Use 2–100 characters: letters, digits, spaces and . _ / -')
      return
    }
    const key = CUSTOM_PREFIX + ua.toLowerCase()
    if (byKey.has(key)) {
      setCustomError('This User-Agent already has a rule.')
      return
    }
    setCustomError(null)
    setCustomUA('')
    setRule(key, ua, 'deny', 0)
  }

  return (
    <div className="space-y-5">
      {CATEGORIES.map(cat => {
        const bots = catalog.filter(b => b.category === cat.id)
        if (bots.length === 0) return null
        return (
          <section key={cat.id}>
            <h3 className="text-sm font-semibold text-gray-900">{cat.label}</h3>
            <p className="text-xs text-gray-500 mb-2">{cat.hint}</p>
            <div className="border border-gray-200 rounded-lg divide-y divide-gray-100">
              {bots.map(bot => (
                <BotRow
                  key={bot.key}
                  name={bot.name}
                  token={bot.pattern}
                  rule={byKey.get(bot.key)}
                  inherited={inheritedByKey.get(bot.key)}
                  rateTiers={rateTiers}
                  disabled={disabled}
                  onSet={(action, rate) => setRule(bot.key, '', action, rate)}
                />
              ))}
            </div>
          </section>
        )
      })}

      <section>
        <h3 className="text-sm font-semibold text-gray-900">Custom rules</h3>
        <p className="text-xs text-gray-500 mb-2">
          Matches any request whose User-Agent contains the text (case-insensitive).
        </p>
        {customKeys.length > 0 && (
          <div className="border border-gray-200 rounded-lg divide-y divide-gray-100 mb-3">
            {customKeys.map(key => {
              const own = byKey.get(key)
              const inh = inheritedByKey.get(key)
              const ua = own?.user_agent || inh?.user_agent || key.slice(CUSTOM_PREFIX.length)
              return (
                <BotRow
                  key={key}
                  name={ua}
                  token={ua}
                  rule={own}
                  inherited={inh}
                  rateTiers={rateTiers}
                  disabled={disabled}
                  onSet={(action, rate) => setRule(key, ua, action, rate)}
                  onRemove={own && !inh ? () => onChange(rules.filter(r => r.bot_key !== key)) : undefined}
                />
              )
            })}
          </div>
        )}
        {!disabled && (
          <>
            <div className="flex flex-wrap gap-2">
              <input
                type="text"
                value={customUA}
                onChange={e => { setCustomUA(e.target.value); setCustomError(null) }}
                onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); addCustom() } }}
                placeholder="User-Agent contains… e.g. MyScraper/1.0"
                aria-label="Custom User-Agent"
                maxLength={100}
                className="flex-1 min-w-48 text-sm font-mono px-3 py-2 border border-gray-200 rounded-lg bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
              />
              <button
                type="button"
                onClick={addCustom}
                className="flex items-center gap-1.5 px-3 py-2 text-sm text-gray-700 border border-gray-200 rounded-lg hover:bg-gray-50"
              >
                <Plus size={15} /> Add rule
              </button>
            </div>
            {customError && <p className="text-xs text-red-600 mt-1.5">{customError}</p>}
          </>
        )}
      </section>
    </div>
  )
}
