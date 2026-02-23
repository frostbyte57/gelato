import type { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import { useAppState } from "../lib/store";

export function AppShell(props: { title: string; subtitle?: string; children: ReactNode }) {
  const ws = useAppState((s) => s.ws);
  const wsError = useAppState((s) => s.wsError);
  const hello = useAppState((s) => s.hello);

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-icon">G</div>
          <div>
            <div className="brand-title">gelato</div>
            <div className="brand-subtitle">Local Log Console</div>
          </div>
        </div>
        <nav className="nav">
          <NavLink to="/" end className={({ isActive }) => `nav-link ${isActive ? "active" : ""}`}>
            Dashboard
          </NavLink>
          <NavLink to="/connections" className={({ isActive }) => `nav-link ${isActive ? "active" : ""}`}>
            Connections
          </NavLink>
        </nav>
        <div className="sidebar-footer">
          <div className={`pill ${ws}`}>WS: {ws}</div>
          {hello && <div className="meta-line">flush {hello.flushMs}ms · full {hello.fullSnapshotSec}s</div>}
          {wsError && <div className="error-text">{wsError}</div>}
        </div>
      </aside>

      <main className="main-panel">
        <header className="topbar">
          <div>
            <h1>{props.title}</h1>
            {props.subtitle && <p>{props.subtitle}</p>}
          </div>
        </header>
        <section className="page-content">{props.children}</section>
      </main>
    </div>
  );
}
