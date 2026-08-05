// Typed REST client for cmd/api.
// Base URL is the same origin in prod (Next.js routes /api/v1/* → cmd/api
// via the rewrites in next.config.mjs).

const BASE = "/api/v1";

export class ApiError extends Error {
  status: number;
  code?: string;
  body: unknown;

  constructor(status: number, body: unknown, message?: string) {
    super(message ?? `API ${status}`);
    this.status = status;
    this.body = body;
    const code = (body as { error?: { code?: string } } | null)?.error?.code;
    if (code) this.code = code;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
    credentials: "include",
  });

  if (!res.ok) {
    let body: unknown = null;
    const text = await res.text();
    try {
      body = text ? JSON.parse(text) : null;
    } catch {
      body = text;
    }
    throw new ApiError(res.status, body);
  }

  if (res.status === 204) {
    return undefined as T;
  }
  return (await res.json()) as T;
}

export const api = {
  get: <T>(path: string) => request<T>(path, { method: "GET" }),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: "POST", body: body ? JSON.stringify(body) : undefined }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: "PATCH", body: body ? JSON.stringify(body) : undefined }),
  delete: <T>(path: string) => request<T>(path, { method: "DELETE" }),
};

// ----- Types mirroring the Go API -----

export type User = {
  id: string;
  email: string;
  role: "admin" | "member";
  emailVerified: boolean;
};

export type Workspace = {
  id: string;
  slug: string;
  name: string;
  plan: "free" | "pro" | "agency";
  status: "active" | "suspended" | "canceled";
  countryCode: string;
  timezone: string;
  locale: string;
  promptOverride: string | null;
  openaiKeyConfigured: boolean;
  whatsapp: {
    status: "not_linked" | "pending_qr" | "authenticated" | "ready" | "disconnected" | "error";
    phoneNumber: string | null;
  } | null;
};

export type Lead = {
  id: string;
  phone: string;
  name: string;
  status: string;
  stage: string;
  fields: Record<string, string | number | boolean | null>;
  nurturePaused: boolean;
  nurturePausedUntil: string | null;
  lastInboundAt: string | null;
  lastContactedAt: string | null;
  nextFollowupAt: string | null;
  followupCount: number;
  createdAt: string;
  updatedAt: string;
};

export type LeadListItem = Pick<
  Lead,
  "id" | "phone" | "name" | "status" | "stage" | "lastContactedAt" | "lastInboundAt"
>;

export type LeadDetail = {
  lead: Lead;
  history: Message[];
  extractions: Extraction[];
};

export type Message = {
  id: string;
  phone: string;
  direction: "inbound" | "outbound";
  sender: "customer" | "ai" | "counselor";
  body: string;
  messageType: string;
  timestamp: string;
};

export type Extraction = {
  id: string;
  fields: Record<string, unknown>;
  promptTemplate: string | null;
  createdAt: string;
};

export type Destination = {
  id: string;
  type: "google_sheets" | "airtable" | "notion";
  name: string;
  enabled: boolean;
  fieldMapping: Record<string, string>;
  lastSyncAt: string | null;
  lastError: string | null;
  createdAt: string;
};

export type WhatsAppSession = {
  id: string;
  status: "not_linked" | "pending_qr" | "authenticated" | "ready" | "disconnected" | "error";
  phoneNumber: string | null;
  workerId: string | null;
  qrDataUrl: string | null;
  qrExpiresAt: string | null;
  lastError: string | null;
};

export type PromptVersion = {
  template: string;
  firstSeen: string;
  uses: number;
};

export type PromptPreview = {
  compiledPreview: string;
  predictedStage: string;
  languageHint: "english" | "malayalam" | "manglish";
};

export type StageUsage = {
  stage: string;
  count: number;
};

// ----- Constants derived from internal/nurture -----

export const LEAD_STATUSES = [
  "New",
  "Contacted",
  "Interested",
  "Follow-up",
  "Lost",
  "Enrolled",
] as const;
export type LeadStatus = (typeof LEAD_STATUSES)[number];

export const LEAD_STAGES = [
  "ask_name",
  "ask_qualification",
  "ask_pass_fail",
  "ask_stream",
  "ask_failed_subjects",
  "ask_certificate",
  "ask_place",
  "complete",
] as const;
export type LeadStage = (typeof LEAD_STAGES)[number];

export const STAGE_LABELS: Record<LeadStage, string> = {
  ask_name: "Ask name",
  ask_qualification: "Ask qualification",
  ask_pass_fail: "Ask pass/fail",
  ask_stream: "Ask stream",
  ask_failed_subjects: "Ask failed subjects",
  ask_certificate: "Ask certificate",
  ask_place: "Ask place",
  complete: "Complete",
};

export const STAGE_DESCRIPTIONS: Record<LeadStage, string> = {
  ask_name: "First contact. Collect the customer's name.",
  ask_qualification: "Determine class level (10th / 12th / other).",
  ask_pass_fail: "Ask whether they passed or failed the previous exam.",
  ask_stream: "Stream selection (Science / Commerce / Arts / Humanities).",
  ask_failed_subjects: "Count and list of subjects the customer failed.",
  ask_certificate: "Whether they have the original certificate / mark sheet.",
  ask_place: "Place where they want to sit the NIOS exam.",
  complete: "All required fields collected. Nurture sequence finished.",
};
