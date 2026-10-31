import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import Layout from './components/Layout'
import ProtectedRoute from './components/ProtectedRoute'

const FinancePage = lazy(() => import('./pages/FinancePage'))
const ReportsPage = lazy(() => import('./pages/ReportsPage'))
const HomePage = lazy(() => import('./pages/HomePage'))
const FiscalPage = lazy(() => import('./pages/FiscalPage'))
const InventoryPage = lazy(() => import('./pages/InventoryPage'))
const StockMovementsPage = lazy(() => import('./pages/StockMovementsPage'))
const UnifiedImportHistoryPage = lazy(() => import('./pages/UnifiedImportHistoryPage'))
const LoginPage = lazy(() => import('./pages/LoginPage'))
const StaffPage = lazy(() => import('./pages/StaffPage'))
const CustomersPage = lazy(() => import('./pages/CustomersPage'))
const AcceptStaffInvitePage = lazy(() => import('./pages/AcceptStaffInvitePage'))
const ResetStaffPasswordPage = lazy(() => import('./pages/ResetStaffPasswordPage'))
const PDVPage = lazy(() => import('./pages/PDVPage'))
const ProductsPage = lazy(() => import('./pages/ProductsPage'))
const PurchasesPage = lazy(() => import('./pages/PurchasesPage'))
const ReturnsPage = lazy(() => import('./pages/ReturnsPage'))
const SetupPage = lazy(() => import('./pages/SetupPage'))

function RouteLoadingFallback() {
  return (
    <div className="p-6 text-sm text-slate-600" role="status">
      Carregando tela…
    </div>
  )
}

export default function App() {
  return (
    <Suspense fallback={<RouteLoadingFallback />}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/accept-invite" element={<AcceptStaffInvitePage />} />
        <Route path="/reset-password" element={<ResetStaffPasswordPage />} />

        <Route element={<ProtectedRoute />}>
          <Route element={<Layout />}>
            <Route index element={<Navigate to="/home" replace />} />
            <Route path="/home" element={<HomePage />} />
            <Route path="/setup" element={<SetupPage />} />
            <Route path="/staff" element={<StaffPage />} />
            <Route path="/products" element={<ProductsPage />} />
            <Route path="/inventory" element={<InventoryPage />} />
            <Route path="/stock-movements" element={<StockMovementsPage />} />
            <Route path="/imports" element={<UnifiedImportHistoryPage />} />
            <Route path="/purchases" element={<PurchasesPage />} />
            <Route path="/customers" element={<CustomersPage />} />
            <Route path="/returns" element={<ReturnsPage />} />
            <Route path="/pdv" element={<PDVPage />} />
            <Route path="/finance" element={<FinancePage />} />
            <Route path="/reports" element={<ReportsPage />} />
            <Route path="/fiscal" element={<FiscalPage />} />
          </Route>
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
  )
}
