import { useState } from "react";
import { BatchResponse, DEFAULT_TENANT, PricingRequest, runBatch } from "../api";

export default function BatchDashboardPage() {
  const [tenant, setTenant] = useState(DEFAULT_TENANT);
  const [rowsInput, setRowsInput] = useState(
    "SKU-00001,1,web\nSKU-00002,2,store\nSKU-00003,1,mobile"
  );
  const [out, setOut] = useState<BatchResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  const run = async () => {
    setError(null);
    try {
      const requests: PricingRequest[] = rowsInput
        .split("\n")
        .map((l) => l.trim())
        .filter(Boolean)
        .map((l) => {
          const [sku, qtyS, channel] = l.split(",");
          return {
            tenant_id: tenant,
            customer_id: "cust-001",
            sku: sku.trim(),
            quantity: Number(qtyS),
            channel: channel.trim(),
            at: new Date().toISOString(),
          } as PricingRequest;
        });
      const r = await runBatch(tenant, requests);
      setOut(r);
    } catch (e) {
      setError(String(e));
    }
  };

  return (
    <div>
      <h1>Batch dashboard</h1>
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
          <div className="field" style={{ flex: 2 }}>
            <label>Rows (sku, qty, channel per line)</label>
            <textarea rows={5} value={rowsInput} onChange={(e) => setRowsInput(e.target.value)} />
          </div>
        </div>
        <button onClick={run}>Run batch</button>
        {error && <div className="pill danger">Error: {error}</div>}
      </div>

      {out && (
        <div className="card">
          <h2>Result</h2>
          <div className="muted">job_id: {out.job_id} · result_hash: {out.result_hash}</div>
          <table>
            <thead>
              <tr>
                <th>SKU</th>
                <th>Qty</th>
                <th>Channel</th>
                <th>Base</th>
                <th>Final</th>
                <th>Applied</th>
              </tr>
            </thead>
            <tbody>
              {out.result.map((r, i) => (
                <tr key={i}>
                  <td>{r.request.sku}</td>
                  <td>{r.request.quantity}</td>
                  <td>{r.request.channel}</td>
                  <td>{r.decision.base_yen.toLocaleString()}</td>
                  <td>{r.decision.final_yen.toLocaleString()}</td>
                  <td>{r.decision.applied_promotion_ids.join(", ") || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
