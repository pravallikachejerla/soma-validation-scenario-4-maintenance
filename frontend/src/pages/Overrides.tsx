import { useEffect, useState } from "react";
import {
  approveOverrideRequest,
  createOverrideRequest,
  getOverrideSettings,
  listOverrideRequests,
  OverrideRequest,
  rejectOverrideRequest,
  TenantOverrideSettings,
  upsertOverrideSettings,
} from "../override";

const DEFAULT_TENANT = "tenant-acme";

const TEST_IDENTITIES = {
  requester: "alice.requester",
  approver1: "bob.approver",
  approver2: "carol.approver",
  admin: "dave.admin",
};

export default function OverridesPage() {
  const [tenant, setTenant] = useState(DEFAULT_TENANT);
  const [settings, setSettings] = useState<TenantOverrideSettings | null>(null);
  const [requests, setRequests] = useState<OverrideRequest[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);

  // Form state
  const [requesterId, setRequesterId] = useState(TEST_IDENTITIES.requester);
  const [sku, setSku] = useState("SKU-001");
  const [channel, setChannel] = useState("web");
  const [baseYen, setBaseYen] = useState(10000);
  const [overrideYen, setOverrideYen] = useState(8000);
  const [reason, setReason] = useState("VIP customer retention");

  // Approval form state
  const [activeId, setActiveId] = useState<string | null>(null);
  const [approverId, setApproverId] = useState(TEST_IDENTITIES.approver1);
  const [approverReason, setApproverReason] = useState("Approved per policy");

  // Settings form state
  const [thresholdYen, setThresholdYen] = useState(5_000_000);
  const [requiredApprovals, setRequiredApprovals] = useState(2);
  const [overrideEnabled, setOverrideEnabled] = useState(true);

  async function refresh() {
    try {
      setError(null);
      const s = await getOverrideSettings(tenant);
      setSettings(s);
      setThresholdYen(s.threshold_yen);
      setRequiredApprovals(s.required_approvals);
      setOverrideEnabled(s.override_enabled);
      const list = await listOverrideRequests(tenant);
      setRequests(list);
    } catch (e: any) {
      setError(e.message || String(e));
    }
  }

  useEffect(() => {
    refresh();
  }, [tenant]);

  async function onSaveSettings() {
    try {
      setError(null);
      setInfo(null);
      const updated = await upsertOverrideSettings({
        tenant_id: tenant,
        override_enabled: overrideEnabled,
        threshold_yen: thresholdYen,
        required_approvals: requiredApprovals,
      });
      setSettings(updated);
      setInfo("Settings saved.");
    } catch (e: any) {
      setError(e.message || String(e));
    }
  }

  async function onCreate() {
    try {
      setError(null);
      setInfo(null);
      const created = await createOverrideRequest({
        tenant_id: tenant,
        requester_id: requesterId,
        sku,
        channel,
        base_yen: baseYen,
        override_yen: overrideYen,
        reason,
      });
      setInfo(`Created override request ${created.id} (status=${created.status}).`);
      await refresh();
    } catch (e: any) {
      setError(e.message || String(e));
    }
  }

  async function onApprove() {
    if (!activeId) return;
    try {
      setError(null);
      setInfo(null);
      const updated = await approveOverrideRequest(activeId, approverId, approverReason);
      setInfo(`Approved by ${approverId} (status=${updated.status}).`);
      await refresh();
    } catch (e: any) {
      setError(e.message || String(e));
    }
  }

  async function onReject() {
    if (!activeId) return;
    try {
      setError(null);
      setInfo(null);
      const updated = await rejectOverrideRequest(activeId, approverId, approverReason);
      setInfo(`Rejected by ${approverId} (status=${updated.status}).`);
      await refresh();
    } catch (e: any) {
      setError(e.message || String(e));
    }
  }

  return (
    <div className="page">
      <h2>Manual price overrides</h2>
      <p className="muted">
        Per-tenant approval workflow. Default threshold JPY 5,000,000. Two approvals required above the threshold.
        Requester cannot self-approve. Reason is mandatory. Every action is audited.
      </p>

      <section className="card">
        <h3>Tenant</h3>
        <label>
          Tenant ID
          <input value={tenant} onChange={(e) => setTenant(e.target.value)} />
        </label>
      </section>

      <section className="card">
        <h3>Per-tenant settings</h3>
        {settings && (
          <p className="muted">
            Current: enabled={String(settings.override_enabled)}, threshold={settings.threshold_yen} JPY,
            required_approvals={settings.required_approvals}
          </p>
        )}
        <label>
          Override enabled
          <input
            type="checkbox"
            checked={overrideEnabled}
            onChange={(e) => setOverrideEnabled(e.target.checked)}
          />
        </label>
        <label>
          Threshold (JPY)
          <input
            type="number"
            value={thresholdYen}
            onChange={(e) => setThresholdYen(Number(e.target.value))}
          />
        </label>
        <label>
          Required approvals
          <input
            type="number"
            value={requiredApprovals}
            onChange={(e) => setRequiredApprovals(Number(e.target.value))}
          />
        </label>
        <button onClick={onSaveSettings}>Save settings</button>
      </section>

      <section className="card">
        <h3>Create override request</h3>
        <label>
          Requester
          <select value={requesterId} onChange={(e) => setRequesterId(e.target.value)}>
            <option value={TEST_IDENTITIES.requester}>{TEST_IDENTITIES.requester}</option>
            <option value={TEST_IDENTITIES.approver1}>{TEST_IDENTITIES.approver1}</option>
            <option value={TEST_IDENTITIES.approver2}>{TEST_IDENTITIES.approver2}</option>
            <option value={TEST_IDENTITIES.admin}>{TEST_IDENTITIES.admin}</option>
          </select>
        </label>
        <label>
          SKU
          <input value={sku} onChange={(e) => setSku(e.target.value)} />
        </label>
        <label>
          Channel
          <input value={channel} onChange={(e) => setChannel(e.target.value)} />
        </label>
        <label>
          Base (JPY)
          <input type="number" value={baseYen} onChange={(e) => setBaseYen(Number(e.target.value))} />
        </label>
        <label>
          Override (JPY)
          <input type="number" value={overrideYen} onChange={(e) => setOverrideYen(Number(e.target.value))} />
        </label>
        <label>
          Reason (mandatory)
          <input value={reason} onChange={(e) => setReason(e.target.value)} />
        </label>
        <button onClick={onCreate}>Submit override request</button>
      </section>

      <section className="card">
        <h3>Approve / reject</h3>
        <label>
          Request ID
          <input value={activeId ?? ""} onChange={(e) => setActiveId(e.target.value)} placeholder="paste request id" />
        </label>
        <label>
          Approver
          <select value={approverId} onChange={(e) => setApproverId(e.target.value)}>
            <option value={TEST_IDENTITIES.approver1}>{TEST_IDENTITIES.approver1}</option>
            <option value={TEST_IDENTITIES.approver2}>{TEST_IDENTITIES.approver2}</option>
            <option value={TEST_IDENTITIES.admin}>{TEST_IDENTITIES.admin}</option>
          </select>
        </label>
        <label>
          Reason (mandatory)
          <input value={approverReason} onChange={(e) => setApproverReason(e.target.value)} />
        </label>
        <button onClick={onApprove}>Approve</button>
        <button onClick={onReject}>Reject</button>
      </section>

      <section className="card">
        <h3>Requests for {tenant}</h3>
        <button onClick={refresh}>Refresh</button>
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>SKU</th>
              <th>Channel</th>
              <th>Base</th>
              <th>Override</th>
              <th>Status</th>
              <th>Approvals</th>
              <th>Requester</th>
            </tr>
          </thead>
          <tbody>
            {requests.map((r) => (
              <tr key={r.id}>
                <td><code>{r.id}</code></td>
                <td>{r.sku}</td>
                <td>{r.channel}</td>
                <td>{r.base_yen}</td>
                <td>{r.override_yen}</td>
                <td>{r.status}</td>
                <td>
                  {r.approvals.length}/{r.required_count}
                </td>
                <td>{r.requester_id}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      {error && <p className="error">Error: {error}</p>}
      {info && <p className="info">{info}</p>}

      <section className="card">
        <h3>Test identities</h3>
        <ul>
          <li>Requester: <code>{TEST_IDENTITIES.requester}</code></li>
          <li>Approver 1: <code>{TEST_IDENTITIES.approver1}</code></li>
          <li>Approver 2: <code>{TEST_IDENTITIES.approver2}</code></li>
          <li>Administrator: <code>{TEST_IDENTITIES.admin}</code></li>
        </ul>
      </section>
    </div>
  );
}