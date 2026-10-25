import { NavLink, Route, Routes } from "react-router-dom";
import ProductsPage from "./pages/Products";
import PromotionEditorPage from "./pages/PromotionEditor";
import PricingSimulatorPage from "./pages/PricingSimulator";
import BatchDashboardPage from "./pages/BatchDashboard";
import AuditViewerPage from "./pages/AuditViewer";

export default function App() {
  return (
    <div className="app">
      <aside className="sidebar">
        <h1>SOMA Pricing</h1>
        <nav>
          <NavLink to="/products" className={({ isActive }) => (isActive ? "active" : "")}>
            Products
          </NavLink>
          <NavLink to="/promotions" className={({ isActive }) => (isActive ? "active" : "")}>
            Promotions
          </NavLink>
          <NavLink to="/simulator" className={({ isActive }) => (isActive ? "active" : "")}>
            Pricing simulator
          </NavLink>
          <NavLink to="/batch" className={({ isActive }) => (isActive ? "active" : "")}>
            Batch dashboard
          </NavLink>
          <NavLink to="/audit" className={({ isActive }) => (isActive ? "active" : "")}>
            Audit viewer
          </NavLink>
        </nav>
      </aside>
      <main className="main">
        <Routes>
          <Route path="/" element={<ProductsPage />} />
          <Route path="/products" element={<ProductsPage />} />
          <Route path="/promotions" element={<PromotionEditorPage />} />
          <Route path="/simulator" element={<PricingSimulatorPage />} />
          <Route path="/batch" element={<BatchDashboardPage />} />
          <Route path="/audit" element={<AuditViewerPage />} />
        </Routes>
      </main>
    </div>
  );
}
