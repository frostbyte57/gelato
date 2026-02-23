export type ListenerStatus = number;
export type Level = number;

export interface Listener {
  ID: string;
  BindHost: string;
  Port: number;
  Status: ListenerStatus;
  ErrMsg: string;
  ActiveConns: number;
  Dropped: number;
  Errors: number;
  StartedAt: string;
}

export interface Source {
  Key: string;
  Name: string;
  Color: string;
  Muted: boolean;
  LastSeen: string;
  ActiveConns: number;
  Dropped: number;
  Errors: number;
}

export interface StatsSnapshot {
  LogsPerSec: number[];
  ErrorsPerMin: number[];
  DropsPerMin: number[];
}

export interface Snapshot {
  Listeners: Listener[];
  ListenerStats: Record<string, ListenerStats>;
  Sources: Source[];
  SourceStats: Record<string, SourceStats>;
  Lines: string[];
  LineLevels: Level[];
  Dropped: number;
  Errors: number;
  OverflowSources: number;
  ShardDrops: number[];
  SourceErrorRates: Record<string, number[]>;
  SourceDropRates: Record<string, number[]>;
  ListenerErrorRates: Record<string, number[]>;
  ListenerDropRates: Record<string, number[]>;
  Stats: StatsSnapshot;
}

export interface ListenerStats {
  ActiveConns: number;
  Dropped: number;
  Rejected: number;
  Errors: number;
}

export interface SourceStats {
  ActiveConns: number;
  Dropped: number;
  Errors: number;
}

export interface FiltersDTO {
  levelMask: number;
  searchText: string;
  sourceKey: string;
}

export interface APIErrorEnvelope {
  error: { code: string; message: string };
}

export interface LogLineDTO {
  line: string;
  level: Level;
}

export interface ListenerPatch {
  listener: Listener;
  stats: ListenerStats;
}

export interface SourcePatch {
  source: Source;
  stats: SourceStats;
}

export interface DeltaFrame {
  type: "delta";
  fromSeq: number;
  toSeq: number;
  newLogs?: LogLineDTO[];
  listenerPatches?: ListenerPatch[];
  sourcePatches?: SourcePatch[];
  stats: {
    stats: StatsSnapshot;
    dropped: number;
    errors: number;
    overflowSources: number;
    shardDrops: number[];
    listenerErrorRates: Record<string, number[]>;
    listenerDropRates: Record<string, number[]>;
    sourceErrorRates: Record<string, number[]>;
    sourceDropRates: Record<string, number[]>;
  };
  truncateHint?: number;
  flags?: { filtersChanged?: boolean; overflow?: boolean };
}

export interface SnapshotFrame {
  type: "snapshot";
  seq: number;
  reason: string;
  snapshot: Snapshot;
}

export interface HelloFrame {
  type: "hello";
  version: string;
  serverTime: string;
  flushMs: number;
  maxLogBatch: number;
  fullSnapshotSec: number;
}

export interface ResyncRequiredFrame {
  type: "resync_required";
  fromSeq: number;
  currentSeq: number;
  reason: string;
}

export interface ErrorFrame {
  type: "error";
  code: string;
  message: string;
}

export type WsFrame = HelloFrame | SnapshotFrame | DeltaFrame | ResyncRequiredFrame | ErrorFrame;

export interface CreateListenerRequest {
  bindHost: string;
  port: number;
  protocol: "tcp";
}

