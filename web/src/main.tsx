import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import './styles/base.css'
import './styles/rentals.css'
import './styles/app.css'
import './styles/settings.css'
import { AppProvider, useApp } from './lib/app-context'
import { Loading, ToastProvider } from './components/ui'
import Layout from './components/Layout'
import { ErrorBoundary } from './components/ErrorBoundary'
import Login from './pages/Login'
import Today from './pages/Today'
import Rent from './pages/Rent'
import Properties from './pages/Properties'
import PropertyPage from './pages/PropertyPage'
import Leases from './pages/Leases'
import LeasePage from './pages/LeasePage'
import Tenants, { TenantPage } from './pages/Tenants'
import Repairs from './pages/Repairs'
import RepairPage from './pages/RepairPage'
import Inspections from './pages/Inspections'
import InspectionPage from './pages/InspectionPage'
import Costs from './pages/Costs'
import Reports from './pages/Reports'
import Settings from './pages/Settings'

function Gate({ children }: { children: React.ReactNode }) {
  const { status, user } = useApp()
  const loc = useLocation()
  if (!status) return <div className="boot"><img src="/favicon.svg" alt="" /><Loading /></div>
  if (status.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/login" replace state={{ from: loc.pathname + loc.search }} />
  return <>{children}</>
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login mode="login" />} />
      <Route path="/setup" element={<Login mode="setup" />} />
      <Route path="/settings/:tab?" element={<Gate><ErrorBoundary where="settings"><Settings /></ErrorBoundary></Gate>} />
      <Route element={<Gate><ErrorBoundary where="rentals"><Layout /></ErrorBoundary></Gate>}>
        <Route path="/" element={<Today />} />
        <Route path="/rent" element={<Rent />} />
        <Route path="/properties" element={<Properties />} />
        <Route path="/properties/:id" element={<PropertyPage />} />
        <Route path="/leases" element={<Leases />} />
        <Route path="/leases/:id" element={<LeasePage />} />
        <Route path="/tenants" element={<Tenants />} />
        <Route path="/tenants/:id" element={<TenantPage />} />
        <Route path="/repairs" element={<Repairs />} />
        <Route path="/repairs/:id" element={<RepairPage />} />
        <Route path="/inspections" element={<Inspections />} />
        <Route path="/inspections/:id" element={<InspectionPage />} />
        <Route path="/costs" element={<Costs />} />
        <Route path="/reports" element={<Reports />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary where="app"><ToastProvider><AppProvider><App /></AppProvider></ToastProvider></ErrorBoundary>
    </BrowserRouter>
  </React.StrictMode>,
)
