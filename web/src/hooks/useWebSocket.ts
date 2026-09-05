import { useEffect, useRef, useCallback, useState } from 'react'
import type { WebSocketMessage } from '@/types'

const WS_URL = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`

type MessageHandler = (data: unknown) => void

export function useWebSocket(channels: string[], onMessage?: MessageHandler) {
  const wsRef = useRef<WebSocket | null>(null)
  const [connected, setConnected] = useState(false)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const channelsRef = useRef(channels)
  channelsRef.current = channels

  // Callers pass a fresh array literal on every render, so `channels` identity is
  // never stable. Key the resubscribe effect on the contents instead — with the
  // raw array as the dep, the effect fired on EVERY render and sent another
  // `subscribe` frame; the server answers a dashboard:metrics subscribe by
  // replaying its whole metrics cache, each replayed message updates state, and
  // that render resubscribes again. A self-sustaining loop bounded only by network
  // round-trip time, hammering the browser and the API for as long as the tab was
  // open.
  const channelKey = channels.join('|')

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return

    try {
      const ws = new WebSocket(WS_URL)

      ws.onopen = () => {
        setConnected(true)
        // Subscribe to all requested channels
        channelsRef.current.forEach(channel => {
          ws.send(JSON.stringify({ action: 'subscribe', channel }))
        })
      }

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data) as WebSocketMessage
          // Deliberately no `setLastMessage` here. It existed but was never read by
          // any consumer, and forcing a re-render per inbound message was one half
          // of the resubscribe loop described above.
          onMessage?.(msg.data)
        } catch {
          // ignore invalid messages
        }
      }

      ws.onclose = () => {
        setConnected(false)
        // Reconnect after 3 seconds
        reconnectTimer.current = setTimeout(connect, 3000)
      }

      ws.onerror = () => {
        ws.close()
      }

      wsRef.current = ws
    } catch {
      // Connection failed, retry
      reconnectTimer.current = setTimeout(connect, 5000)
    }
  }, [onMessage])

  useEffect(() => {
    connect()

    return () => {
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
    }
  }, [connect])

  // Re-subscribe only when the channel list actually changes (see channelKey).
  useEffect(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      channelsRef.current.forEach(channel => {
        wsRef.current?.send(JSON.stringify({ action: 'subscribe', channel }))
      })
    }
  }, [channelKey])

  return { connected }
}

// Hook specifically for real-time server metrics
export function useServerMetrics(serverIds: number[]) {
  const [metrics, setMetrics] = useState<Record<number, {
    cpu_usage: number
    ram_usage: number
    disk_usage: number
    uptime: number
  }>>({})

  const channels = serverIds.map(id => `server:${id}:metrics`)

  const handleMessage = useCallback((data: unknown) => {
    const d = data as { server_id: number; cpu_usage: number; ram_usage: number; disk_usage: number; uptime: number }
    if (d?.server_id) {
      setMetrics(prev => ({
        ...prev,
        [d.server_id]: {
          cpu_usage: d.cpu_usage,
          ram_usage: d.ram_usage,
          disk_usage: d.disk_usage,
          uptime: d.uptime,
        },
      }))
    }
  }, [])

  const { connected } = useWebSocket(channels, handleMessage)

  return { metrics, connected }
}

type DashboardMetric = {
  server_id: number
  server_name: string
  ip_address: string
  cpu_usage: number
  ram_usage: number
  disk_usage: number
  uptime: number
  status: string
}

type InitialServer = {
  id: number
  name: string
  ip_address: string
  status: string
  cpu_usage?: number
  ram_usage?: number
  disk_usage?: number
}

// Hook for dashboard-level metrics from all servers.
// Accepts optional initialServers (from REST /servers) to pre-populate state
// immediately on mount — avoiding the blank screen while waiting for the first
// WebSocket heartbeat (up to 30s).
export function useDashboardMetrics(initialServers?: InitialServer[]) {
  const [metrics, setMetrics] = useState<Record<number, DashboardMetric>>(() => {
    // Pre-populate from REST server list so metrics show immediately on load.
    // WebSocket updates will overwrite these with live values as they arrive.
    if (!initialServers?.length) return {}
    const initial: Record<number, DashboardMetric> = {}
    for (const s of initialServers) {
      if (s.status === 'active') {
        initial[s.id] = {
          server_id: s.id,
          server_name: s.name,
          ip_address: s.ip_address,
          cpu_usage:  s.cpu_usage  ?? 0,
          ram_usage:  s.ram_usage  ?? 0,
          disk_usage: s.disk_usage ?? 0,
          uptime: 0,
          status: s.status,
        }
      }
    }
    return initial
  })

  // Re-sync whenever the REST server list changes:
  // • Add new active servers that aren't in metrics yet
  // • Remove entries for servers that have been deleted from the panel
  useEffect(() => {
    if (!initialServers) return
    setMetrics(prev => {
      const validIds = new Set(initialServers.filter(s => s.status === 'active').map(s => s.id))
      const next: Record<number, DashboardMetric> = {}
      // Keep only still-valid entries
      for (const [idStr, m] of Object.entries(prev)) {
        if (validIds.has(Number(idStr))) next[Number(idStr)] = m
      }
      // Add any newly active servers
      for (const s of initialServers) {
        if (s.status === 'active' && !next[s.id]) {
          next[s.id] = {
            server_id: s.id,
            server_name: s.name,
            ip_address: s.ip_address,
            cpu_usage:  s.cpu_usage  ?? 0,
            ram_usage:  s.ram_usage  ?? 0,
            disk_usage: s.disk_usage ?? 0,
            uptime: 0,
            status: s.status,
          }
        }
      }
      return next
    })
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialServers?.length])

  // Build an allow-set of known server IDs so WebSocket pushes from stale/
  // removed agents (still running on the VPS) are silently dropped.
  //
  // Held in a ref, not a plain const: handleMessage is memoized (it must be, or
  // the socket would be torn down and reopened on every render), so a captured
  // Set would freeze at the value from the render that created the callback.
  // Because the memo key was the server *count*, deleting one server and adding
  // another left the count unchanged and the callback kept filtering against the
  // old IDs — the new server's metrics were silently dropped. A ref is read at
  // call time, so it is always current.
  const knownServerIds = useRef<Set<number>>(new Set())
  knownServerIds.current = new Set(initialServers?.map(s => s.id) ?? [])

  const handleMessage = useCallback((data: unknown) => {
    const d = data as { server_id: number; server_name: string; ip_address: string; cpu_usage: number; ram_usage: number; disk_usage: number; uptime: number; status: string }
    if (d?.server_id && knownServerIds.current.has(d.server_id)) {
      setMetrics(prev => ({
        ...prev,
        [d.server_id]: d,
      }))
    }
  }, [])

  const { connected } = useWebSocket(['dashboard:metrics'], handleMessage)

  return { metrics, connected }
}
