import { Navigate, Route, Routes } from 'react-router-dom'
import Layout from './components/Layout'
import ProtectedRoute from './components/ProtectedRoute'
import FinancePage from './pages/FinancePage'
import FiscalPage from './pages/FiscalPage'
import InventoryPage from './pages/InventoryPage'
import LoginPage from './pages/LoginPage'
import PDVPage from './pages/PDVPage'
import ProductsPage from './pages/ProductsPage'

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

      <Route element={<ProtectedRoute />}>
        <Route element={<Layout />}>
          <Route index element={<Navigate to="/products" replace />} />
          <Route path="/products" element={<ProductsPage />} />
          <Route path="/inventory" element={<InventoryPage />} />
          <Route path="/pdv" element={<PDVPage />} />
          <Route path="/finance" element={<FinancePage />} />
          <Route path="/fiscal" element={<FiscalPage />} />
        </Route>
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
