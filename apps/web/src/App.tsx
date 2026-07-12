import { Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { HomePage } from './pages/HomePage'
import { LoginPage } from './pages/LoginPage'
import { NotePage } from './pages/NotePage'
import { TrashPage } from './pages/TrashPage'
import { SettingsPage } from './pages/SettingsPage'
import { useVaultEvents } from './hooks/useVaultEvents'
import { useAuthStore } from './store/auth'

export default function App() {
  useVaultEvents()
  const unauthorized = useAuthStore((s) => s.unauthorized)

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
