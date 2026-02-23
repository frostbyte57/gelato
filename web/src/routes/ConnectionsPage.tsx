import { FormEvent, useMemo, useState } from "react";
import { AppShell } from "../components/AppShell";
import { createListener, deleteListener, retryListener } from "../lib/api";
import { useAppState } from "../lib/store";

function statusLabel(status: number) {
  switch (status) {
    case 1:
      return "listening";
    case 2:
      return "error";
    case 3:
      return "stopped";
    default:
      return "starting";
  }
}

export function ConnectionsPage() {
  const snapshot = useAppState((s) => s.snapshot);
  const [search, setSearch] = useState("");
  const [host, setHost] = useState("127.0.0.1");
  const [port, setPort] = useState(9000);
  const [busy, setBusy] = useState<string>("");
  const [err, setErr] = useState("");

  const listeners = useMemo(() => {
    const q = search.trim().toLowerCase();
    const base = snapshot?.Listeners ?? [];
    if (!q) return base;
    return base.filter((l) =>
      `${l.ID} ${l.BindHost}:${l.Port}`.toLowerCase().includes(q),
    );
  }, [snapshot, search]);

  async function onCreate(e: FormEvent) {
    e.preventDefault();
    setBusy("create");
    setErr("");
    try {
      await createListener({ bindHost: host, port: Number(port), protocol: "tcp" });
    } catch (error) {
      setErr(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy("");
    }
  }

  async function onDelete(id: string) {
    setBusy(id);
    setErr("");
    try {
      await deleteListener(id);
    } catch (error) {
      setErr(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy("");
    }
  }

  async function onRetry(id: string) {
    setBusy(`retry:${id}`);
    setErr("");
    try {
      await retryListener(id);
    } catch (error) {
      setErr(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy("");
    }
  }

  return (
    <AppShell title="Connections Management" subtitle="Add and manage TCP listeners (any valid host/IP and port)">
      <div className="grid connections-layout">
        <div className="card">
          <div className="panel-header">
            <h3>Add Connection</h3>
            <span className="muted">TCP only (UDP/HTTP not implemented yet)</span>
          </div>
          <form className="form-grid" onSubmit={onCreate}>
            <label>
              Bind Host / IP
              <input className="input" value={host} onChange={(e) => setHost(e.target.value)} placeholder="127.0.0.1" />
            </label>
            <label>
              Port
              <input
                className="input"
                type="number"
                min={1}
                max={65535}
                value={port}
                onChange={(e) => setPort(Number(e.target.value))}
              />
            </label>
            <label>
              Protocol
              <select className="select" value="tcp" disabled>
                <option value="tcp">TCP (supported)</option>
                <option value="udp">UDP (not implemented)</option>
                <option value="http">HTTP (not implemented)</option>
              </select>
            </label>
            <div className="form-actions">
              <button className="button" disabled={busy === "create"} type="submit">
                {busy === "create" ? "Creating..." : "Add Connection"}
              </button>
            </div>
          </form>
          {err && <div className="error-banner">{err}</div>}
        </div>

        <div className="card">
          <div className="panel-header">
            <h3>Listeners</h3>
            <span className="muted">{listeners.length} shown</span>
          </div>
          <div className="toolbar slim">
            <input className="input" placeholder="Search listeners..." value={search} onChange={(e) => setSearch(e.target.value)} />
          </div>
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Bind</th>
                  <th>Status</th>
                  <th>Conns</th>
                  <th>Dropped</th>
                  <th>Rejected</th>
                  <th>Errors</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {listeners.map((l) => {
                  const stats = snapshot?.ListenerStats[l.ID];
                  return (
                    <tr key={l.ID}>
                      <td>{l.ID}</td>
                      <td>{l.BindHost}:{l.Port}</td>
                      <td><span className={`status-chip ${statusLabel(l.Status)}`}>{statusLabel(l.Status)}</span></td>
                      <td>{stats?.ActiveConns ?? 0}</td>
                      <td>{stats?.Dropped ?? 0}</td>
                      <td>{stats?.Rejected ?? 0}</td>
                      <td>{stats?.Errors ?? 0}</td>
                      <td className="actions">
                        <button className="button tiny secondary" disabled={busy !== ""} onClick={() => void onRetry(l.ID)}>Retry</button>
                        <button className="button tiny danger" disabled={busy !== ""} onClick={() => void onDelete(l.ID)}>Remove</button>
                      </td>
                    </tr>
                  );
                })}
                {listeners.length === 0 && (
                  <tr>
                    <td colSpan={8} className="empty-cell">No listeners yet</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </AppShell>
  );
}

