import { GeoAccessCard } from '@/components/geo/GeoAccessCard'
import { useSiteGeoRules, useUpdateSiteGeoRules } from '@/hooks/useGeo'

export function GeoAccessPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data, isLoading } = useSiteGeoRules(siteId)
  const update = useUpdateSiteGeoRules(siteId)

  return (
    <GeoAccessCard
      scope="site"
      data={data}
      isLoading={isLoading}
      isSaving={update.isPending}
      canManage={canManage}
      onSave={(inherit, rule) => update.mutateAsync({ inherit, ...rule })}
    />
  )
}
