import { BotControlCard } from '@/components/bots/BotControlCard'
import { useServerBotRules, useUpdateServerBotRules } from '@/hooks/useBots'

export function BotControlTab({ serverId, canManage }: { serverId: number; canManage: boolean }) {
  const { data, isLoading } = useServerBotRules(serverId)
  const update = useUpdateServerBotRules(serverId)

  return (
    <BotControlCard
      description="Default bot rules for every site on this server. Each site can override individual bots or opt out in its Security tab."
      switchLabel="Enable server defaults"
      switchHelp={on => on
        ? 'These rules apply to every site that inherits server defaults.'
        : 'Rules are saved but not applied to any site.'}
      switchField="enabled"
      data={data}
      isLoading={isLoading}
      isSaving={update.isPending}
      canManage={canManage}
      onSave={(enabled, rules) => update.mutateAsync({ enabled, rules })}
    />
  )
}
