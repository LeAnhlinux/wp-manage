import React, { useState } from 'react'
import { Check, Copy, Loader2 } from 'lucide-react'

export function StatCard({ icon, label, value, sub }: { icon: React.ReactNode; label: string; value: string; sub?: string }) {
  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-4">
      <div className="flex items-center gap-2 text-gray-400 mb-2">
        {icon}
        <span className="text-xs font-medium uppercase">{label}</span>
      </div>
      <p className="text-lg font-semibold text-gray-900">{value}</p>
      {sub && <p className="text-xs text-gray-400 mt-0.5">{sub}</p>}
    </div>
  )
}

export function DetailRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between py-2 border-b border-gray-50 last:border-0">
      <dt className="text-sm text-gray-500">{label}</dt>
      <dd className="text-sm font-medium text-gray-900">{value}</dd>
    </div>
  )
}

export function CredRow({ label, value, field, copiedField, onCopy, isLink }: {
  label: string; value: string; field: string; copiedField: string
  onCopy: (text: string, field: string) => void; isLink?: boolean
}) {
  const isCopied = copiedField === field
  return (
    <div className="flex items-center justify-between py-2">
      <span className="text-sm text-gray-500">{label}</span>
      <div className="flex items-center gap-2">
        {isLink ? (
          <a href={value} target="_blank" rel="noopener" className="text-sm font-mono text-blue-600 hover:underline">
            {value}
          </a>
        ) : (
          <code className="text-sm font-mono bg-gray-50 px-2 py-0.5 rounded text-gray-800">{value}</code>
        )}
        <button onClick={() => onCopy(value, field)}
          className="p-1 text-gray-400 hover:text-blue-600 rounded">
          {isCopied ? <Check size={14} className="text-green-500" /> : <Copy size={14} />}
        </button>
      </div>
    </div>
  )
}

export function TabButton({ active, onClick, icon, label, 'data-tab': dataTab }: {
  active: boolean; onClick: () => void; icon: React.ReactNode; label: string; 'data-tab'?: string
}) {
  return (
    <button
      onClick={onClick}
      data-tab={dataTab}
      className={`flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded-lg whitespace-nowrap flex-shrink-0 transition-all duration-150 ${
        active
          ? 'bg-blue-600 text-white shadow-sm'
          : 'text-gray-500 hover:text-gray-800 hover:bg-gray-100'
      }`}
    >
      {icon} {label}
    </button>
  )
}

export function TabDivider() {
  return <div className="w-px h-5 bg-gray-200 mx-0.5 flex-shrink-0" />
}

export function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)
  const handleCopy = () => {
    navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }
  return (
    <button onClick={handleCopy} title="Copy"
      className="p-1 text-gray-400 hover:text-blue-600 flex-shrink-0">
      {copied ? <Check size={13} className="text-green-500" /> : <Copy size={13} />}
    </button>
  )
}

export function ActionBtn({ icon, label, onClick, loading, color }: {
  icon: React.ReactNode; label: string; onClick: () => void; loading: boolean; color: string
}) {
  const colors: Record<string, string> = {
    blue: 'bg-blue-50 text-blue-700 hover:bg-blue-100',
    yellow: 'bg-yellow-50 text-yellow-700 hover:bg-yellow-100',
    green: 'bg-green-50 text-green-700 hover:bg-green-100',
    red: 'bg-red-50 text-red-700 hover:bg-red-100',
  }
  return (
    <button onClick={onClick} disabled={loading}
      className={`w-full flex items-center gap-2 px-4 py-2.5 text-sm font-medium rounded-lg disabled:opacity-50 transition-colors ${colors[color]}`}>
      {loading ? <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-current" /> : icon}
      {label}
    </button>
  )
}
