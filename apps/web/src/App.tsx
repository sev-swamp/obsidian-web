import { useEffect } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { HomePage } from './pages/HomePage'
import { LoginPage } from './pages/LoginPage'
import { NotePage } from './pages/NotePage'
import { TrashPage } from './pages/TrashPage'
import { SettingsPage } from './pages/SettingsPage'
import { useVaultEvents } from './hooks/useVaultEvents'
import { useAuthStore } from './store/auth'
import { api } from './api/client'
import { startSync, wakeSync } from './sync/runtime'

export default function App() {
  useVaultEvents()
  const unauthorized = useAuthStore((s) => s.unauthorized)

  useEffect(() => {
    let enabled = false
    void api.settings().then((settings) => {
      enabled = settings.sync?.enabled === true
      localStorage.setItem('obsidianweb-sync-enabled', enabled ? '1' : '0')
      localStorage.setItem('obsidianweb-default-folder', settings.notes.defaultFolder || '')
      if (enabled) void startSync()
    }).catch(() => undefined)
    if (localStorage.getItem('obsidianweb-sync-enabled') === '1') { enabled = true; void startSync() }
    const online = () => { if (enabled) wakeSync() }
    window.addEventListener('online', online)
    return () => window.removeEventListener('online', online)
  }, [])

  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={unauthorized ? <Navigate to="/login" replace /> : <Layout />}
      >
        <Route path="/" element={<HomePage />} />
        <Route path="/n/*" element={<NotePage />} />
        <Route path="/trash" element={<TrashPage />} />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>
    </Routes>
  )
}
