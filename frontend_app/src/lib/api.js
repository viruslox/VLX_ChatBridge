// All URLs are relative (no leading slash) so the SPA works when mounted under
// an Apache subpath in front of the frontend server. Vite's base "./" makes the
// document base match the mount point.

export async function getStatus() {
  const r = await fetch("api/status");
  if (!r.ok) throw new Error("status request failed: " + r.status);
  return r.json();
}

export function toggleModule(name, enabled) {
  return post("api/module", { name, enabled });
}

export function toggleFeature(key, enabled) {
  return post("api/feature", { key, enabled });
}

export function shutdown() {
  return post("api/shutdown", {});
}

export async function consoleTicket() {
  const r = await fetch("api/console/ticket");
  if (!r.ok) throw new Error("console ticket failed: " + r.status);
  const data = await r.json();
  return data.ticket;
}

async function post(url, body) {
  const r = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!r.ok) {
    let msg = "request failed: " + r.status;
    try {
      const e = await r.json();
      if (e && e.error) msg = e.error;
    } catch (_) {
      /* ignore parse error */
    }
    throw new Error(msg);
  }
  return r.json();
}
