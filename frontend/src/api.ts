export interface Product {
  id: string;
  tenant_id: string;
  sku: string;
  name: string;
  base_yen: number;
}

export interface Promotion {
  id: string;
  tenant_id: string;
  name: string;
  type: "percent" | "amount";
  value: number;
  channel: string;
  sku?: string;
  priority: number;
  valid_from: string;
  valid_until: string;
}

export interface PricingRequest {
  tenant_id: string;
  customer_id: string;
  sku: string;
  quantity: number;
  channel: string;
  at?: string;
}

export interface PricingDecision {
  request_hash: string;
  base_yen: number;
  final_yen: number;
  applied_promotion_ids: string[];
  audit_ids: string[];
  explanation_lines: string[];
}

export interface AuditEvent {
  id: string;
  tenant_id: string;
  action: string;
  subject: string;
  detail: string;
  created_at: string;
}

async function jsonOrThrow<T>(r: Response): Promise<T> {
  if (!r.ok) {
    const text = await r.text();
    throw new Error(text || `HTTP ${r.status}`);
  }
  return r.json() as Promise<T>;
}

export async function listProducts(tenant: string): Promise<Product[]> {
  return jsonOrThrow<Product[]>(await fetch(`/api/v1/products?tenant_id=${encodeURIComponent(tenant)}`));
}

export async function listPromotions(tenant: string, channel: string): Promise<Promotion[]> {
  return jsonOrThrow<Promotion[]>(
    await fetch(`/api/v1/promotions?tenant_id=${encodeURIComponent(tenant)}&channel=${encodeURIComponent(channel)}`)
  );
}

export async function createPromotion(p: Promotion): Promise<Promotion> {
  return jsonOrThrow<Promotion>(
    await fetch("/api/v1/promotions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(p),
    })
  );
}

export async function patchPromotion(id: string, p: Partial<Promotion>): Promise<Promotion> {
  return jsonOrThrow<Promotion>(
    await fetch(`/api/v1/promotions/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(p),
    })
  );
}

export async function quote(req: PricingRequest): Promise<PricingDecision> {
  return jsonOrThrow<PricingDecision>(
    await fetch("/api/v1/pricing/quote", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    })
  );
}

export interface BatchResultRow {
  request: PricingRequest;
  decision: PricingDecision;
}

export interface BatchResponse {
  job_id: string;
  result: BatchResultRow[];
  result_hash: string;
}

export async function runBatch(tenant: string, requests: PricingRequest[]): Promise<BatchResponse> {
  return jsonOrThrow<BatchResponse>(
    await fetch("/api/v1/pricing/batch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ tenant_id: tenant, requests }),
    })
  );
}

export async function listAudit(tenant: string, limit = 50): Promise<AuditEvent[]> {
  return jsonOrThrow<AuditEvent[]>(
    await fetch(`/api/v1/audit?tenant_id=${encodeURIComponent(tenant)}&limit=${limit}`)
  );
}

export const DEFAULT_TENANT = "tenant-a";
