// React Query key factory. Centralizing keys prevents typos and makes
// invalidation painless.

export const qk = {
  me: () => ["me"] as const,
  workspace: () => ["workspace"] as const,
  leads: (filter: { status?: string; search?: string; page?: number } = {}) =>
    ["leads", filter] as const,
  lead: (id: string) => ["lead", id] as const,
  destinations: () => ["destinations"] as const,
  whatsapp: () => ["whatsapp-session"] as const,
  promptDefault: () => ["prompt-default"] as const,
  promptVersions: () => ["prompt-versions"] as const,
  stageUsage: () => ["stage-usage"] as const,
  overview: () => ["overview"] as const,
  apiKeys: () => ["api-keys"] as const,
};
