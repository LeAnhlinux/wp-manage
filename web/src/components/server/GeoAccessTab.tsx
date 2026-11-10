import { GeoAccessCard } from '@/components/geo/GeoAccessCard'
import { useServerGeoRules, useUpdateServerGeoRules } from '@/hooks/useGeo'

export function GeoAccessTab({ serverId, canManage }: { serverId: number; canManage: boolean }) {
  const { data, isLoading } = useServerGeoRules(serverId)
  const update = useUpdateServerGeoRules(serverId)

  return (
    <GeoAccessCard
      scope="server"
      data={data}
      isLoading={isLoading}
      isSaving={update.isPending}
      canManage={canManage}
      onSave={(enabled, rule) => update.mutateAsync({ enabled, ...rule })}
    />
  )
}
