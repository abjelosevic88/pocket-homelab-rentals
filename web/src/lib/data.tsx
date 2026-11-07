// The person's places, totals and app states, shared by every page.
import React, { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, V1 } from './api'
import type { AppState, Property, Summary } from './types'

const EVENT = 'prt:changed'

interface DataState {
  properties: Property[] | null
  summary: Summary | null
  apps: AppState[] | null
  reload: () => Promise<void>
  reloadApps: () => Promise<void>
}
const Ctx = createContext<DataState>({ properties: null, summary: null, apps: null, reload: async () => {}, reloadApps: async () => {} })
export function useData() { return useContext(Ctx) }

/** Something changed (a payment, a lease): lists and counts reload. */
export function announceChange() { window.dispatchEvent(new CustomEvent(EVENT)) }

export function useOnChange(fn: () => void) {
  useEffect(() => {
    window.addEventListener(EVENT, fn)
    return () => window.removeEventListener(EVENT, fn)
  }, [fn])
}

export function DataProvider({ children }: { children: React.ReactNode }) {
  const [properties, setProperties] = useState<Property[] | null>(null)
  const [summary, setSummary] = useState<Summary | null>(null)
  const [apps, setApps] = useState<AppState[] | null>(null)
  const reload = useCallback(async () => {
    const [ps, sum] = await Promise.all([api.get<Property[]>(`${V1}/properties`).catch(() => null), api.get<Summary>(`${V1}/summary`).catch(() => null)])
    setProperties(p => ps || p || [])
    if (sum) setSummary(sum)
  }, [])
  const reloadApps = useCallback(async () => {
    try { setApps((await api.get<{ apps: AppState[] }>(`${V1}/apps`)).apps) } catch { setApps(a => a || []) }
  }, [])
  useEffect(() => { reload(); reloadApps() }, [reload, reloadApps])
  useOnChange(reload)
  useEffect(() => {
    const t = window.setInterval(() => { if (document.visibilityState === 'visible') { reload(); reloadApps() } }, 120000)
    return () => window.clearInterval(t)
  }, [reload, reloadApps])
  return <Ctx.Provider value={{ properties, summary, apps, reload, reloadApps }}>{children}</Ctx.Provider>
}

/** One app's state for this person; `ok` = the scope works. */
export function useAppState() {
  const { apps } = useData()
  return (slug: string, scope?: string) => {
    const a = apps?.find(x => x.slug === slug)
    const ok = !!a && a.via !== '' && (!scope || a.on.includes(scope))
    return { app: a, ok, loading: !apps }
  }
}
