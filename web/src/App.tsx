import { Navigate, Route, Routes } from 'react-router-dom'
import Layout from './components/Layout'
import ProtectedRoute from './components/ProtectedRoute'
import FinancePage from './pages/FinancePage'
import HomePage from './pages/HomePage'
import FiscalPage from './pages/FiscalPage'
import InventoryPage from './pages/InventoryPage'
import UnifiedImportHistoryPage from './pages/UnifiedImportHistoryPage'
import LoginPage from './pages/LoginPage'
import StaffPage from './pages/StaffPage'
import AcceptStaffInvitePage from './pages/AcceptStaffInvitePage'
import PDVPage from './pages/PDVPage'
import ProductsPage from './pages/ProductsPage'
import PurchasesPage from './pages/PurchasesPage'
import ReturnsPage from './pages/ReturnsPage'
import SetupPage from './pages/SetupPage'

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/accept-invite" element={<AcceptStaffInvitePage />} />

      <Route element={<ProtectedRoute />}>
        <Route element={<Layout />}>
          <Route index element={<Navigate to="/home" replace />} />
          <Route path="/home" element={<HomePage />} />
          <Route path="/setup" element={<SetupPage />} />
          <Route path="/staff" element={<StaffPage />} />
          <Route path="/products" element={<ProductsPage />} />
          <Route path="/inventory" element={<InventoryPage />} />
          <Route path="/imports" element={<UnifiedImportHistoryPage />} />
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
