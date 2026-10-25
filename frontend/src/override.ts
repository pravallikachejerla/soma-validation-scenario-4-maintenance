// Override workflow types and API client.
export interface TenantOverrideSettings {
  tenant_id: string;
  override_enabled: boolean;
  threshold_yen: number;
  required_approvals: number;
}

export interface Approval {
  approver_id: string;
  decision: "approve" | "reject";
  reason: string;
  decided_at: string;
}

export interface OverrideRequest {
  id: string;
  tenant_id: string;
  requester_id: string;
  sku: string;
  channel: string;
  override_yen: number;
  base_yen: number;
  reason: string;
  status: "pending" | "approved" | "rejected" | "cancelled" | "applied";
  approvals: Approval[];
  required_count: number;
  created_at: string;
  updated_at: string;
  applied_at?: string;
}

async function jsonOrThrow<T>(r: Response): Promise<T> {
  if (!r.ok) {
    const text = await r.text();
    throw new Error(text || `HTTP ${r.status}`);
  }
  return r.json() as Promise<T>;
}

export async function getOverrideSettings(tenant: string): Promise<TenantOverrideSettings> {
  return jsonOrThrow<TenantOverrideSettings>(
    await fetch(`/api/v1/overrides/settings?tenant_id=${encodeURIComponent(tenant)}`)
  );
}

export async function upsertOverrideSettings(s: TenantOverrideSettings): Promise<TenantOverrideSettings> {
  return jsonOrThrow<TenantOverrideSettings>(
    await fetch("/api/v1/overrides/settings", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(s),
    })
  );
}

export async function createOverrideRequest(r: Omit<OverrideRequest, "id" | "status" | "approvals" | "required_count" | "created_at" | "updated_at">): Promise<OverrideRequest> {
  return jsonOrThrow<OverrideRequest>(
    await fetch("/api/v1/overrides/requests", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(r),
    })
  );
}

export async function listOverrideRequests(tenant: string): Promise<OverrideRequest[]> {
  return jsonOrThrow<OverrideRequest[]>(
    await fetch(`/api/v1/overrides/requests?tenant_id=${encodeURIComponent(tenant)}`)
  );
}

export async function getOverrideRequest(id: string): Promise<OverrideRequest> {
  return jsonOrThrow<OverrideRequest>(
    await fetch(`/api/v1/overrides/requests/${encodeURIComponent(id)}`)
  );
}

export async function approveOverrideRequest(id: string, approver_id: string, reason: string): Promise<OverrideRequest> {
  return jsonOrThrow<OverrideRequest>(
    await fetch(`/api/v1/overrides/requests/${encodeURIComponent(id)}/approve`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ approver_id, reason }),
    })
  );
}

export async function rejectOverrideRequest(id: string, approver_id: string, reason: string): Promise<OverrideRequest> {
  return jsonOrThrow<OverrideRequest>(
    await fetch(`/api/v1/overrides/requests/${encodeURIComponent(id)}/reject`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ approver_id, reason }),
    })
  );
}