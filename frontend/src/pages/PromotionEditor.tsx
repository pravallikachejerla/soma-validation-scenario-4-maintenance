import { useEffect, useState } from "react";
import { DEFAULT_TENANT, Promotion, createPromotion, listPromotions, patchPromotion } from "../api";

const CHANNELS = ["web", "store", "mobile"];

export default function PromotionEditorPage() {
  const [tenant, setTenant] = useState(DEFAULT_TENANT);
  const [channel, setChannel] = useState("web");
  const [items, setItems] = useState<Promotion[]>([]);
  const [draft, setDraft] = useState<Partial<Promotion>>({
    type: "percent",
    value: 10,
    channel: "web",
    sku: "",
    priority: 1,
  });
  const [error, setError] = useState<string | null>(null);
  const [savedId, setSavedId] = useState<string | null>(null);

  const reload = () => {
    listPromotions(tenant, channel)
      .then(setItems)
      .catch((e) => setError(String(e)));
  };

  useEffect(reload, [tenant, channel]);

  const save = async () => {
    setError(null);
    try {
      const body: Promotion = {
        id: "",
        tenant_id: tenant,
        name: draft.name || `Promotion ${Date.now()}`,
        type: (draft.type as Promotion["type"]) || "percent",
        value: Number(draft.value || 0),
        channel: draft.channel || "web",
        sku: draft.sku || "",
        priority: Number(draft.priority || 0),
        valid_from: draft.valid_from || "2026-01-01T00:00:00Z",
        valid_until: draft.valid_until || "2027-01-01T00:00:00Z",
      };
      const created = await createPromotion(body);
      setSavedId(created.id);
      reload();
    } catch (e) {
      setError(String(e));
    }
  };

  const setPriority = async (p: Promotion, delta: number) => {
    try {
      await patchPromotion(p.id, { ...p, priority: p.priority + delta });
      reload();
    } catch (e) {
      setError(String(e));
    }
  };

  return (
    <div>
      <h1>Promotion editor</h1>
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
            <label>Channel</label>
            <select value={channel} onChange={(e) => setChannel(e.target.value)}>
              {CHANNELS.map((c) => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
          </div>
        </div>
      </div>

      <div className="card">
        <h2>New promotion</h2>
        <div className="row">
          <div className="field">
            <label>Name</label>
            <input value={draft.name ?? ""} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
          </div>
          <div className="field">
            <label>Type</label>
            <select value={draft.type} onChange={(e) => setDraft({ ...draft, type: e.target.value as Promotion["type"] })}>
              <option value="percent">percent</option>
              <option value="amount">amount</option>
            </select>
          </div>
          <div className="field">
            <label>Value</label>
            <input type="number" value={draft.value ?? 0} onChange={(e) => setDraft({ ...draft, value: Number(e.target.value) })} />
          </div>
        </div>
        <div className="row">
          <div className="field">
            <label>Channel</label>
            <select value={draft.channel} onChange={(e) => setDraft({ ...draft, channel: e.target.value })}>
              {CHANNELS.map((c) => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
          </div>
          <div className="field">
            <label>SKU (empty for wildcard)</label>
            <input value={draft.sku ?? ""} onChange={(e) => setDraft({ ...draft, sku: e.target.value })} />
          </div>
          <div className="field">
            <label>Priority</label>
            <input type="number" value={draft.priority ?? 0} onChange={(e) => setDraft({ ...draft, priority: Number(e.target.value) })} />
          </div>
        </div>
        <button onClick={save}>Create</button>
        {savedId && <span className="muted"> saved as {savedId}</span>}
        {error && <div className="pill danger">Error: {error}</div>}
      </div>

      <div className="card">
        <h2>Existing promotions ({items.length})</h2>
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Name</th>
              <th>Type</th>
              <th>Value</th>
              <th>Channel</th>
              <th>SKU</th>
              <th>Priority</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {items.slice(0, 30).map((p) => (
              <tr key={p.id + p.sku}>
                <td>{p.id}</td>
                <td>{p.name}</td>
                <td>{p.type}</td>
                <td>{p.value}</td>
                <td>{p.channel}</td>
                <td>{p.sku || "—"}</td>
                <td>{p.priority}</td>
                <td>
                  <button className="secondary" onClick={() => setPriority(p, 1)}>+1</button>
                  {" "}
                  <button className="secondary" onClick={() => setPriority(p, -1)}>-1</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
