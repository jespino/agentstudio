/** The `server.proxy` entries for the backend's endpoints (/rpc, /events and routes). */
export function studioProxy(options: { backend: string; routes?: string[] }): Record<
  string,
  { target: string; changeOrigin: boolean; headers: Record<string, string> }
>;
