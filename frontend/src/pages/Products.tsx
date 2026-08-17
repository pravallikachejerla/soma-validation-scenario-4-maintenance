import { useEffect, useState } from "react";
import { DEFAULT_TENANT, Product, listProducts } from "../api";

export default function ProductsPage() {
  const [tenant, setTenant] = useState(DEFAULT_TENANT);
  const [items, setItems] = useState<Product[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    listProducts(tenant)
      .then(setItems)
      .catch((e) => setError(String(e)));
  }, [tenant]);

  return (
    <div>
      <h1>Products</h1>
      <div className="card">
        <div className="field">
          <label>Tenant</label>
          <select value={tenant} onChange={(e) => setTenant(e.target.value)}>
            <option value="tenant-a">tenant-a</option>
            <option value="tenant-b">tenant-b</option>
            <option value="tenant-c">tenant-c</option>
          </select>
        </div>
        {error && <div className="pill danger">Error: {error}</div>}
        <table>
          <thead>
            <tr>
              <th>SKU</th>
              <th>Name</th>
              <th>Base (yen)</th>
            </tr>
          </thead>
          <tbody>
            {items.slice(0, 50).map((p) => (
              <tr key={p.id}>
                <td>{p.sku}</td>
                <td>{p.name}</td>
                <td>{p.base_yen.toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {items.length > 50 && (
          <div className="muted">Showing first 50 of {items.length} products.</div>
        )}
      </div>
    </div>
  );
}
