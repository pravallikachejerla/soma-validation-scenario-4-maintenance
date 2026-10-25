import { useEffect, useState } from "react";
import { AuditEvent, DEFAULT_TENANT, listAudit } from "../api";

export default function AuditViewerPage() {
  const [tenant, setTenant] = useState(DEFAULT_TENANT);
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    listAudit(tenant, 50)
      .then(setEvents)
      .catch((e) => setError(String(e)));
  }, [tenant]);

  return (
    <div>
      <h1>Audit viewer</h1>
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
              <th>Time</th>
              <th>Action</th>
              <th>Subject</th>
              <th>Detail</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e) => (
              <tr key={e.id}>
                <td>{new Date(e.created_at).toLocaleString()}</td>
                <td><span className="pill muted">{e.action}</span></td>
                <td>{e.subject}</td>
                <td className="muted">{e.detail}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
