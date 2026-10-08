import { Navigate, Route, Routes } from 'react-router-dom'
import Layout from './components/Layout'
import ProtectedRoute from './components/ProtectedRoute'
import FinancePage from './pages/FinancePage'
import HomePage from './pages/HomePage'
import FiscalPage from './pages/FiscalPage'
import InventoryPage from './pages/InventoryPage'
import LoginPage from './pages/LoginPage'
import PDVPage from './pages/PDVPage'
import ProductsPage from './pages/ProductsPage'
import PurchasesPage from './pages/PurchasesPage'
import ReturnsPage from './pages/ReturnsPage'

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

      <Route element={<ProtectedRoute />}>
        <Route element={<Layout />}>
          <Route index element={<Navigate to="/home" replace />} />
          <Route path="/home" element={<HomePage />} />
          <Route path="/products" element={<ProductsPage />} />
          <Route path="/inventory" element={<InventoryPage />} />
          <Route path="/purchases" element={<PurchasesPage />} />
          <Route path="/returns" element={<ReturnsPage />} />
          <Route path="/pdv" element={<PDVPage />} />
          <Route path="/finance" element={<FinancePage />} />
          <Route path="/fiscal" element={<FiscalPage />} />
        </Route>
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
