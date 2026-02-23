import { startTransition, useSyncExternalStore } from "react";
import { fetchSnapshot } from "./api";
import type {
  DeltaFrame,
  HelloFrame,
  Listener,
  ListenerPatch,
  Snapshot,
  SnapshotFrame,
  Source,
  SourcePatch,
  WsFrame,
} from "../types/api";

type ConnectionState = "connecting" | "connected" | "reconnecting" | "disconnected";

type AppState = {
  snapshot: Snapshot | null;
  seq: number;
  ws: ConnectionState;
  wsError: string;
  hello: HelloFrame | null;
  lastFrameAt: number | null;
};

const MAX_CLIENT_LINES = 5000;

class AppStore {
  private state: AppState = {
    snapshot: null,
    seq: 0,
    ws: "connecting",
    wsError: "",
    hello: null,
    lastFrameAt: null,
  };

  private listeners = new Set<() => void>();
  private started = false;
  private socket: WebSocket | null = null;
  private reconnectTimer: number | null = null;
  private reconnectAttempt = 0;

  start() {
    if (this.started) return;
    this.started = true;
    this.connect();
  }

  subscribe = (cb: () => void) => {
    this.listeners.add(cb);
    return () => {
      this.listeners.delete(cb);
    };
  };

  getSnapshot = () => this.state;

  async forceSnapshot() {
    try {
      const snap = await fetchSnapshot();
      this.applySnapshot({ type: "snapshot", seq: this.state.seq + 1, reason: "manual", snapshot: snap });
    } catch (err) {
      this.patch({ wsError: err instanceof Error ? err.message : String(err) });
    }
  }

  private patch(next: Partial<AppState>) {
    this.state = { ...this.state, ...next };
    this.listeners.forEach((l) => l());
  }

  private connect() {
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsURL = `${proto}//${window.location.host}/api/v1/ws`;
    this.patch({
      ws: this.reconnectAttempt > 0 ? "reconnecting" : "connecting",
      wsError: "",
    });

    const ws = new WebSocket(wsURL);
    this.socket = ws;

    ws.onopen = () => {
      this.reconnectAttempt = 0;
      this.patch({ ws: "connected", wsError: "" });
    };

    ws.onmessage = (event) => {
      try {
        const frame = JSON.parse(event.data as string) as WsFrame;
        this.handleFrame(frame);
      } catch (err) {
        this.patch({ wsError: err instanceof Error ? err.message : "invalid websocket payload" });
      }
    };

    ws.onerror = () => {
      this.patch({ wsError: "websocket error" });
    };

    ws.onclose = () => {
      this.patch({ ws: "disconnected" });
      this.scheduleReconnect();
    };
  }

  private scheduleReconnect() {
    if (this.reconnectTimer != null) return;
    this.reconnectAttempt += 1;
    const base = Math.min(5000, 250 * 2 ** Math.min(this.reconnectAttempt, 4));
    const jitter = Math.floor(Math.random() * 200);
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, base + jitter);
  }

  private handleFrame(frame: WsFrame) {
    if (frame.type === "hello") {
      this.patch({ hello: frame, lastFrameAt: Date.now() });
      return;
    }
    if (frame.type === "error") {
      this.patch({ wsError: `${frame.code}: ${frame.message}`, lastFrameAt: Date.now() });
      return;
    }
    if (frame.type === "resync_required") {
      void this.forceSnapshot();
      return;
    }

    startTransition(() => {
      if (frame.type === "snapshot") {
        this.applySnapshot(frame);
        return;
      }
      this.applyDelta(frame);
    });
  }

  private applySnapshot(frame: SnapshotFrame) {
    if (frame.seq <= this.state.seq) return;
    let snap = frame.snapshot;
    if (snap.Lines.length > MAX_CLIENT_LINES) {
      const start = snap.Lines.length - MAX_CLIENT_LINES;
      snap = {
        ...snap,
        Lines: snap.Lines.slice(start),
        LineLevels: snap.LineLevels.slice(start),
      };
    }
    this.patch({
      snapshot: snap,
      seq: frame.seq,
      lastFrameAt: Date.now(),
      wsError: "",
    });
  }

  private applyDelta(frame: DeltaFrame) {
    if (frame.toSeq <= this.state.seq) return;
    const current = this.state.snapshot;
    if (!current) {
      this.patch({ seq: frame.toSeq, lastFrameAt: Date.now() });
      void this.forceSnapshot();
      return;
    }

    let lines = current.Lines;
    let levels = current.LineLevels;
    if (frame.newLogs && frame.newLogs.length > 0) {
      lines = [...lines, ...frame.newLogs.map((l) => l.line)];
      levels = [...levels, ...frame.newLogs.map((l) => l.level)];
      if (lines.length > MAX_CLIENT_LINES) {
        const start = lines.length - MAX_CLIENT_LINES;
        lines = lines.slice(start);
        levels = levels.slice(start);
      }
    }

    const next: Snapshot = {
      ...current,
      Lines: lines,
      LineLevels: levels,
      Stats: frame.stats.stats,
      Dropped: frame.stats.dropped,
      Errors: frame.stats.errors,
      OverflowSources: frame.stats.overflowSources,
      ShardDrops: frame.stats.shardDrops,
      ListenerErrorRates: frame.stats.listenerErrorRates,
      ListenerDropRates: frame.stats.listenerDropRates,
      SourceErrorRates: frame.stats.sourceErrorRates,
      SourceDropRates: frame.stats.sourceDropRates,
      Listeners: applyListenerPatches(current.Listeners, frame.listenerPatches),
      ListenerStats: applyPatchMap(current.ListenerStats, frame.listenerPatches, (p) => [p.listener.ID, p.stats]),
      Sources: applySourcePatches(current.Sources, frame.sourcePatches),
      SourceStats: applyPatchMap(current.SourceStats, frame.sourcePatches, (p) => [p.source.Key, p.stats]),
    };

    this.patch({
      snapshot: next,
      seq: frame.toSeq,
      lastFrameAt: Date.now(),
      wsError: "",
    });
  }
}

function applyPatchMap<TPatch, TValue>(
  current: Record<string, TValue>,
  patches: TPatch[] | undefined,
  mapper: (patch: TPatch) => [string, TValue],
): Record<string, TValue> {
  if (!patches || patches.length === 0) return current;
  const next = { ...current };
  for (const patch of patches) {
    const [key, val] = mapper(patch);
    next[key] = val;
  }
  return next;
}

function applyListenerPatches(current: Listener[], patches?: ListenerPatch[]): Listener[] {
  if (!patches || patches.length === 0) return current;
  const byID = new Map(current.map((item) => [item.ID, item] as const));
  for (const patch of patches) byID.set(patch.listener.ID, patch.listener);
  return Array.from(byID.values());
}

function applySourcePatches(current: Source[], patches?: SourcePatch[]): Source[] {
  if (!patches || patches.length === 0) return current;
  const byKey = new Map(current.map((item) => [item.Key, item] as const));
  for (const patch of patches) byKey.set(patch.source.Key, patch.source);
  return Array.from(byKey.values());
}

export const appStore = new AppStore();

export function useAppState<T>(selector: (state: AppState) => T): T {
  return useSyncExternalStore(appStore.subscribe, () => selector(appStore.getSnapshot()));
}

