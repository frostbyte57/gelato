import { useDeferredValue, useEffect, useMemo, useState } from "react";
import { AppShell } from "../components/AppShell";
import { VirtualLogList } from "../components/VirtualLogList";
import { patchFilters } from "../lib/api";
import { appStore, useAppState } from "../lib/store";

const LEVEL_MASKS: Record<string, number> = {
  all: 15,
  debug: 1,
  info: 2,
  warn: 4,
  error: 8,
};

function last(values: number[] | undefined): number {
  if (!values || values.length === 0) return 0;
  return values[values.length - 1];
}

export function DashboardPage() {
  const snapshot = useAppState((s) => s.snapshot);
  const hello = useAppState((s) => s.hello);
  const [follow, setFollow] = useState(true);
  const [search, setSearch] = useState("");
  const [sourceKey, setSourceKey] = useState("");
  const [level, setLevel] = useState<keyof typeof LEVEL_MASKS>("all");
  const deferredSearch = useDeferredValue(search);
  const sourceOptions = snapshot?.Sources ?? [];

  useEffect(() => {
    const t = window.setTimeout(() => {
      void patchFilters({
        levelMask: LEVEL_MASKS[level],
        searchText: deferredSearch,
        sourceKey,
      }).catch(() => {});
    }, 150);
    return () => window.clearTimeout(t);
  }, [deferredSearch, level, sourceKey]);

  const kpis = useMemo(() => {
    if (!snapshot) return { logs: 0, errors: 0, drops: 0 };
    return {
      logs: last(snapshot.Stats.LogsPerSec),
      errors: last(snapshot.Stats.ErrorsPerMin),
      drops: last(snapshot.Stats.DropsPerMin),
    };
  }, [snapshot]);

  return (
    <AppShell title="Log Monitoring" subtitle="Real-time ingestion dashboard powered by WebSocket snapshots + deltas">
      <div className="grid cards-3">
        <div className="card kpi">
          <div className="kpi-label">Logs / sec</div>
          <div className="kpi-value">{kpis.logs}</div>
          <div className="kpi-meta">{hello ? `flush ${hello.flushMs}ms` : "waiting for ws"}</div>
        </div>
        <div className="card kpi">
          <div className="kpi-label">Errors / min</div>
          <div className="kpi-value">{kpis.errors}</div>
          <div className="kpi-meta">Total errors {snapshot?.Errors ?? 0}</div>
        </div>
        <div className="card kpi">
          <div className="kpi-label">Drops / min</div>
          <div className="kpi-value">{kpis.drops}</div>
          <div className="kpi-meta">Total drops {snapshot?.Dropped ?? 0}</div>
        </div>
      </div>

      <div className="card toolbar">
        <div className="toolbar-left">
          <input
            className="input"
            placeholder="Search logs..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <select className="select" value={level} onChange={(e) => setLevel(e.target.value as keyof typeof LEVEL_MASKS)}>
            <option value="all">All severities</option>
            <option value="error">Error</option>
            <option value="warn">Warn</option>
            <option value="info">Info</option>
            <option value="debug">Debug</option>
          </select>
          <select className="select" value={sourceKey} onChange={(e) => setSourceKey(e.target.value)}>
            <option value="">All sources</option>
            {sourceOptions.map((source) => (
              <option key={source.Key} value={source.Key}>
                {source.Name || source.Key}
              </option>
            ))}
          </select>
        </div>
        <div className="toolbar-right">
          <button className="button secondary" onClick={() => setFollow((v) => !v)}>
            {follow ? "Pause Follow" : "Resume Follow"}
          </button>
          <button className="button" onClick={() => void appStore.forceSnapshot()}>
            Refresh Snapshot
          </button>
        </div>
      </div>

      <div className="grid main-grid">
        <div className="card">
          <div className="panel-header">
            <h3>Recent Logs</h3>
            <span className="muted">{snapshot?.Lines.length ?? 0} visible lines</span>
          </div>
          <VirtualLogList snapshot={snapshot} follow={follow} />
        </div>

        <div className="stack">
          <div className="card">
            <div className="panel-header">
              <h3>Listeners</h3>
              <span className="muted">{snapshot?.Listeners.length ?? 0}</span>
            </div>
            <div className="compact-table">
              {(snapshot?.Listeners ?? []).slice(0, 8).map((listener) => (
                <div className="compact-row" key={listener.ID}>
                  <div>
                    <div className="row-title">{listener.ID}</div>
                    <div className="muted">{listener.BindHost}:{listener.Port}</div>
                  </div>
                  <div className="align-right">
                    <div>{snapshot?.ListenerStats[listener.ID]?.ActiveConns ?? 0} conns</div>
                    <div className="muted">
                      err {snapshot?.ListenerStats[listener.ID]?.Errors ?? 0} · drop {snapshot?.ListenerStats[listener.ID]?.Dropped ?? 0}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>

          <div className="card">
            <div className="panel-header">
              <h3>Sources</h3>
              <span className="muted">{snapshot?.Sources.length ?? 0}</span>
            </div>
            <div className="compact-table">
              {(snapshot?.Sources ?? []).slice(0, 8).map((source) => (
                <div className="compact-row" key={source.Key}>
                  <div>
                    <div className="row-title">{source.Name || source.Key}</div>
                    <div className="muted">{source.Key}</div>
                  </div>
                  <div className="align-right">
                    <div>{source.ActiveConns} conns</div>
                    <div className="muted">err {source.Errors} · drop {source.Dropped}</div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </AppShell>
  );
}

