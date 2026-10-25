import { useEffect, useState } from "react";
import {
  DEFAULT_TENANT,
  OverrideRequest,
  createOverride,
  decideOverride,
  getTenant,
  listOverrides,
  patchTenant,
} from "../api";

const ROLES = {
  requester: "tenant-a-requester",
  approver: "tenant-a-approver",
  admin: "tenant-a-admin",
};

export default function OverridesPage() {
  const [tenant, setTenant] = useState<string>(DEFAULT_TENANT);
  const [tenantCfg, setTenantCfg] = useState<{
    override_enabled: boolean;
    override_threshold_yen: number;
  } | null>(null);
  const [overrides, setOverrides] = useState<OverrideRequest[]>([]);
  const [error, setError] = useState<string>("");
  const [info, setInfo] = useState<string>("");

  // Form state for creating a new override request.
  const [requesterId, setRequesterId] = useState<string>(ROLES.requester);
  const [sku, setSku] = useState<string>("SKU-00001");
  const [channel, setChannel] = useState<string>("web");
  const [customerId, setCustomerId] = useState<string>("cust-001");
  const [originalYen, setOriginalYen] = useState<number>(1000);
  const [overrideYen, setOverrideYen] = useState<number>(500);
  const [reason, setReason] = useState<string>("");

  // Approver form state.
  const [approverId, setApproverId] = useState<string>(ROLES.approver);
  const [decisionReason, setDecisionReason] = useState<string>("");

  const refresh = async () => {
    try {
      const t = await getTenant(tenant);
      setTenantCfg({
        override_enabled: t.override_enabled,
        override_threshold_yen: t.override_threshold_yen,
      });
      const list = await listOverrides(tenant, 50);
      setOverrides(list);
      setError("");
    } catch (e: any) {
      setError(String(e?.message ?? e));
    }
  };

  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tenant]);

  const onCreate = async () => {
    setError("");
    setInfo("");
    try {
      const r = await createOverride({
        tenant_id: tenant,
        requester_id: requesterId,
        sku,
        channel,
        customer_id: customerId,
        original_yen: originalYen,
        override_yen: overrideYen,
        reason,
      });
      setInfo(`Created override ${r.id} (status=${r.status}, required=${r.required_approvals})`);
      setReason("");
      await refresh();
    } catch (e: any) {
      setError(String(e?.message ?? e));
    }
  };

  const onDecide = async (id: string, action: "approve" | "reject" | "apply") => {
    setError("");
    setInfo("");
    try {
      const r = await decideOverride(id, action, {
        approver_id: approverId,
        reason: decisionReason,
      });
      setInfo(`Override ${id} -> ${r.status}`);
      setDecisionReason("");
      await refresh();
    } catch (e: any) {
      setError(String(e?.message ?? e));
    }
  };

  const onToggleFeature = async () => {
    if (!tenantCfg) return;
    try {
      await patchTenant(tenant, { override_enabled: !tenantCfg.override_enabled });
      await refresh();
    } catch (e: any) {
      setError(String(e?.message ?? e));
    }
  };

  const onSetThreshold = async (yen: number) => {
    try {
      await patchTenant(tenant, { override_threshold_yen: yen });
      await refresh();
    } catch (e: any) {
      setError(String(e?.message ?? e));
    }
  };

  const effectiveThreshold =
    tenantCfg && tenantCfg.override_threshold_yen > 0
      ? tenantCfg.override_threshold_yen
      : 5_000_000;

  return (
    <div className="page">
      <h2>Manual price overrides</h2>
      <p className="muted">
        Tenant: <code>{tenant}</code>. Effective threshold:{" "}
        <strong>¥{effectiveThreshold.toLocaleString()}</strong>. Feature:{" "}
        <strong>{tenantCfg?.override_enabled ? "enabled" : "disabled"}</strong>.
      </p>

      <section className="card">
        <h3>Tenant configuration</h3>
        <p className="muted">
          The override threshold is configurable per tenant. The default is JPY 5,000,000.
          Existing tenant behaviour is unchanged until the feature is enabled.
        </p>
        <div className="row">
          <button onClick={onToggleFeature} aria-label="Toggle override feature">
            {tenantCfg?.override_enabled ? "Disable feature" : "Enable feature"}
          </button>
          <button onClick={() => onSetThreshold(3_000_000)} aria-label="Set threshold 3M">
            Set threshold ¥3,000,000
          </button>
          <button onClick={() => onSetThreshold(0)} aria-label="Clear threshold">
            Clear threshold (use default)
          </button>
        </div>
      </section>

      <section className="card">
        <h3>Create override request</h3>
        <div className="form-grid">
          <label>
            Requester
            <select value={requesterId} onChange={(e) => setRequesterId(e.target.value)}>
              <option value={ROLES.requester}>{ROLES.requester}</option>
              <option value={ROLES.approver}>{ROLES.approver}</option>
              <option value={ROLES.admin}>{ROLES.admin}</option>
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
            Customer
            <input value={customerId} onChange={(e) => setCustomerId(e.target.value)} />
          </label>
          <label>
            Original ¥
            <input
              type="number"
              value={originalYen}
              onChange={(e) => setOriginalYen(Number(e.target.value))}
            />
          </label>
          <label>
            Override ¥
            <input
              type="number"
              value={overrideYen}
              onChange={(e) => setOverrideYen(Number(e.target.value))}
            />
          </label>
          <label className="full">
            Reason (mandatory)
            <input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="e.g. negotiated enterprise deal"
            />
          </label>
        </div>
        <button onClick={onCreate} aria-label="Create override request">
          Create override request
        </button>
      </section>

      <section className="card">
        <h3>Approve / reject / apply</h3>
        <p className="muted">
          A requester cannot approve their own request. An approval reason is mandatory.
        </p>
        <div className="form-grid">
          <label>
            Approver
            <select value={approverId} onChange={(e) => setApproverId(e.target.value)}>
              <option value={ROLES.requester}>{ROLES.requester}</option>
              <option value={ROLES.approver}>{ROLES.approver}</option>
              <option value={ROLES.admin}>{ROLES.admin}</option>
            </select>
          </label>
          <label className="full">
            Decision reason (mandatory)
            <input
              value={decisionReason}
              onChange={(e) => setDecisionReason(e.target.value)}
              placeholder="e.g. price consistent with negotiated terms"
            />
          </label>
        </div>
      </section>

      {error && (
        <div className="error" role="alert">
          {error}
        </div>
      )}
      {info && (
        <div className="info" role="status">
          {info}
        </div>
      )}

      <section className="card">
        <h3>Override requests</h3>
        {overrides.length === 0 && <p className="muted">No override requests yet.</p>}
        <table className="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>Status</th>
              <th>Required</th>
              <th>Approvals</th>
              <th>SKU</th>
              <th>Channel</th>
              <th>Original ¥</th>
              <th>Override ¥</th>
              <th>Reason</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {overrides.map((o) => (
              <tr key={o.id}>
                <td><code>{o.id}</code></td>
                <td>{o.status}</td>
                <td>{o.required_approvals}</td>
                <td>
                  {o.approvals.length === 0
                    ? "—"
                    : o.approvals
                        .map((a) => `${a.approver_id}:${a.decision}`)
                        .join(", ")}
                </td>
                <td>{o.sku}</td>
                <td>{o.channel}</td>
                <td>{o.original_yen.toLocaleString()}</td>
                <td>{o.override_yen.toLocaleString()}</td>
                <td>{o.reason}</td>
                <td>
                  {o.status === "pending" && (
                    <>
                      <button onClick={() => onDecide(o.id, "approve")} aria-label={`Approve ${o.id}`}>
                        Approve
                      </button>{" "}
                      <button onClick={() => onDecide(o.id, "reject")} aria-label={`Reject ${o.id}`}>
                        Reject
                      </button>
                    </>
                  )}
                  {o.status === "approved" && (
                    <button onClick={() => onDecide(o.id, "apply")} aria-label={`Apply ${o.id}`}>
                      Apply
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </div>
  );
}