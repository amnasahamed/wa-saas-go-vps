// Date / phone / token formatting helpers shared across the UI.

export function formatDate(iso: string | null | undefined, fallback = "—"): string {
  if (!iso) return fallback;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return fallback;
  return d.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "2-digit",
  });
}

export function formatDateTime(iso: string | null | undefined, fallback = "—"): string {
  if (!iso) return fallback;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return fallback;
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function formatTime(iso: string | null | undefined, fallback = "—"): string {
  if (!iso) return fallback;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return fallback;
  return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

export function formatRelative(iso: string | null | undefined, now = Date.now()): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "—";
  const diff = (t - now) / 1000;
  const abs = Math.abs(diff);
  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  if (abs < 60) return rtf.format(Math.round(diff), "seconds");
  if (abs < 3600) return rtf.format(Math.round(diff / 60), "minutes");
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), "hours");
  if (abs < 86400 * 30) return rtf.format(Math.round(diff / 86400), "days");
  if (abs < 86400 * 365) return rtf.format(Math.round(diff / (86400 * 30)), "months");
  return rtf.format(Math.round(diff / (86400 * 365)), "years");
}

export function formatPhone(phone: string | null | undefined, fallback = "—"): string {
  if (!phone) return fallback;
  // E.164-ish: +91 98765 43210
  const cleaned = phone.replace(/[^\d+]/g, "");
  if (!cleaned.startsWith("+")) return phone;
  const cc = cleaned.slice(0, 3);
  const rest = cleaned.slice(3);
  if (rest.length === 10) return `${cc} ${rest.slice(0, 5)} ${rest.slice(5)}`;
  return `${cc} ${rest}`;
}

export function approxTokens(text: string): number {
  // Rough heuristic: ~4 chars per token for English.
  return Math.ceil(text.length / 4);
}

export function truncate(s: string, max = 80): string {
  if (s.length <= max) return s;
  return s.slice(0, max - 1) + "…";
}
