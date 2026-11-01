// Vite development proxy for an agent studio: the UI runs in Vite, the Go
// backend in `<app> serve`. The backend wants its session token; both get
// it from AGENTSTUDIO_TOKEN (see the apps' `make dev`).

/**
 * The `server.proxy` entries for the backend's endpoints.
 * @param {{ backend: string, routes?: string[] }} options
 */
export function studioProxy({ backend, routes = [] }) {
  const token = process.env.AGENTSTUDIO_TOKEN;
  const target = {
    target: backend,
    // the backend only answers its own Host
    changeOrigin: true,
    headers: token ? { 'X-Agentstudio-Token': token } : {},
  };
  return Object.fromEntries(['/rpc', '/events', ...routes].map((p) => [p, target]));
}
