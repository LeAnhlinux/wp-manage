import { useEffect, useRef, useCallback, useState } from 'react'
import type { WebSocketMessage } from '@/types'

const WS_URL = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`

type MessageHandler = (data: unknown) => void

export function useWebSocket(channels: string[], onMessage?: MessageHandler) {
  const wsRef = useRef<WebSocket | null>(null)
  const [connected, setConnected] = useState(false)
  const [lastMessage, setLastMessage] = useState<WebSocketMessage | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const channelsRef = useRef(channels)
  channelsRef.current = channels

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
          setLastMessage(msg)
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

  // Re-subscribe when channels change
  useEffect(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      channels.forEach(channel => {
        wsRef.current?.send(JSON.stringify({ action: 'subscribe', channel }))
      })
    }
  }, [channels])

  return { connected, lastMessage }
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

// Hook for dashboard-level metrics from all servers
export function useDashboardMetrics() {
  const [metrics, setMetrics] = useState<Record<number, {
    server_id: number
    server_name: string
    ip_address: string
    cpu_usage: number
    ram_usage: number
    disk_usage: number
    uptime: number
    status: string
  }>>({})

  const handleMessage = useCallback((data: unknown) => {
    const d = data as { server_id: number; server_name: string; ip_address: string; cpu_usage: number; ram_usage: number; disk_usage: number; uptime: number; status: string }
    if (d?.server_id) {
      setMetrics(prev => ({
        ...prev,
        [d.server_id]: d,
      }))
    }
  }, [])

  const { connected } = useWebSocket(['dashboard:metrics'], handleMessage)

  return { metrics, connected }
}
