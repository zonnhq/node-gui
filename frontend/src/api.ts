import type { CacheState, Config, State } from "./types";

// The local daemon serves live data on port 9090. Using an absolute base means
// the dashboard works both inside the Wails webview and in a plain browser.
const BASE = "http://localhost:9090";

export async function fetchState(): Promise<State> {
  const res = await fetch(`${BASE}/api/state`);
  if (!res.ok) throw new Error(`state request failed: ${res.status}`);
  return res.json();
}

export async function fetchConfig(): Promise<Config> {
  const res = await fetch(`${BASE}/api/config`);
  if (!res.ok) throw new Error(`config request failed: ${res.status}`);
  return res.json();
}

export async function saveConfig(cfg: Config): Promise<Config> {
  const res = await fetch(`${BASE}/api/config`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(cfg),
  });
  if (!res.ok) throw new Error(`config save failed: ${res.status}`);
  return res.json();
}

// enroll sends a one time code from Zonn Console to the daemon, which exchanges
// it for this machine's credential. The credential never reaches this page.
export async function enroll(code: string): Promise<State> {
  const res = await fetch(`${BASE}/api/enroll`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code }),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || `enrollment failed: ${res.status}`);
  return body as State;
}

export async function reportIssue(level: string, message: string): Promise<void> {
  const res = await fetch(`${BASE}/api/report`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ level, message }),
  });
  if (!res.ok) throw new Error(`report failed: ${res.status}`);
}

// updateAction asks the daemon to check for a newer Zonn Node now ("check"), or
// to download, verify, and open its installer ("install").
export async function updateAction(action: "check" | "install"): Promise<void> {
  const res = await fetch(`${BASE}/api/update`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action }),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || `update failed: ${res.status}`);
}

// clearCache removes every downloaded model no running stage needs.
export async function clearCache(): Promise<CacheState> {
  const res = await fetch(`${BASE}/api/cache`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action: "clear" }),
  });
  if (!res.ok) throw new Error(`clearing the cache failed: ${res.status}`);
  return res.json();
}
