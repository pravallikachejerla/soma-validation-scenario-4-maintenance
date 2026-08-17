import { useState } from "react";
import { DEFAULT_TENANT, PricingDecision, quote } from "../api";

const CHANNELS = ["web", "store", "mobile"];

export default function PricingSimulatorPage() {
  const [tenant, setTenant] = useState(DEFAULT_TENANT);
  const [customer, setCustomer] = useState("cust-001");
  const [sku, setSku] = useState("SKU-00001");
  const [qty, setQty] = useState(1);
  const [channel, setChannel] = useState("web");
  const [out, setOut] = useState<PricingDecision | null>(null);
  const [error, setError] = useState<string | null>(null);

  const run = async () => {
    setError(null);
    try {
      const d = await quote({
        tenant_id: tenant,
        customer_id: customer,
        sku,
        quantity: qty,
        channel,
        at: new Date().toISOString(),
      });
      setOut(d);
    } catch (e) {
      setError(String(e));
    }
  };

  return (
    <div>
      <h1>Pricing simulator</h1>
      <div className="card">
        <div className="row">
          <div className="field">
            <label>Tenant</label>
            <select value={tenant} onChange={(e) => setTenant(e.target.value)}>
              <option value="tenant-a">tenant-a</option>
              <option value="tenant-b">tenant-b</option>
              <option value="tenant-c">tenant-c</option>
            </select>
          </div>
          <div className="field">
            <label>Customer</label>
            <input value={customer} onChange={(e) => setCustomer(e.target.value)} />
          </div>
          <div className="field">
            <label>SKU</label>
            <input value={sku} onChange={(e) => setSku(e.target.value)} />
          </div>
        </div>
        <div className="row">
          <div className="field">
            <label>Quantity</label>
            <input type="number" value={qty} onChange={(e) => setQty(Number(e.target.value))} />
          </div>
          <div className="field">
            <label>Channel</label>
            <select value={channel} onChange={(e) => setChannel(e.target.value)}>
              {CHANNELS.map((c) => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
          </div>
        </div>
        <button onClick={run}>Run quote</button>
        {error && <div className="pill danger">Error: {error}</div>}
      </div>

      {out && (
        <div className="card">
          <h2>Decision</h2>
          <table>
            <tbody>
              <tr><th>Base yen</th><td>{out.base_yen.toLocaleString()}</td></tr>
              <tr><th>Final yen</th><td>{out.final_yen.toLocaleString()}</td></tr>
              <tr><th>Applied promotions</th><td>{out.applied_promotion_ids.join(", ") || "(none)"}</td></tr>
              <tr><th>Audit ids</th><td>{out.audit_ids.join(", ") || "(none)"}</td></tr>
            </tbody>
          </table>
          <h2>Explanation</h2>
          <ul>
            {out.explanation_lines.map((l, i) => (
              <li key={i}>{l}</li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
