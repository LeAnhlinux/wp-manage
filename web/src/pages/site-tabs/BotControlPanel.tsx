import { BotControlCard } from '@/components/bots/BotControlCard'
import { useSiteBotRules, useUpdateSiteBotRules } from '@/hooks/useBots'

export function BotControlPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data, isLoading } = useSiteBotRules(siteId)
  const update = useUpdateSiteBotRules(siteId)

  return (
    <BotControlCard
      description="Allow, block or rate-limit crawlers on this site by User-Agent. Rules set here override the server defaults for the same bot."
      switchLabel="Inherit server defaults"
      switchHelp={on => on
        ? 'Bots left on “Default” follow the server-wide rules.'
        : 'Only the rules set on this site apply.'}
      switchField="inherit"
      data={data}
      isLoading={isLoading}
      isSaving={update.isPending}
      canManage={canManage}
      onSave={(inherit, rules) => update.mutateAsync({ inherit, rules })}
      notice={on => on && data && !data.server_enabled ? (
        <p className="text-xs text-gray-500 mb-4">
          Server defaults are currently turned off for this server, so nothing is inherited.
        </p>
      ) : null}
    />
  )
}
