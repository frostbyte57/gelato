import { useEffect, useMemo, useRef, useState } from "react";
import type { Snapshot } from "../types/api";

const ROW_HEIGHT = 22;

function levelName(level: number): string {
  switch (level) {
    case 4:
      return "ERROR";
    case 3:
      return "WARN";
    case 2:
      return "INFO";
    case 1:
      return "DEBUG";
    default:
      return "UNK";
  }
}

export function VirtualLogList(props: { snapshot: Snapshot | null; follow: boolean }) {
  const rootRef = useRef<HTMLDivElement | null>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const lines = props.snapshot?.Lines ?? [];
  const levels = props.snapshot?.LineLevels ?? [];

  useEffect(() => {
    if (!props.follow) return;
    const el = rootRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [lines.length, props.follow]);

  const viewportHeight = 360;
  const totalHeight = lines.length * ROW_HEIGHT;
  const visibleCount = Math.ceil(viewportHeight / ROW_HEIGHT) + 8;
  const start = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - 4);
  const end = Math.min(lines.length, start + visibleCount);

  const items = useMemo(() => {
    const out: Array<{ idx: number; text: string; level: number }> = [];
    for (let i = start; i < end; i++) {
      out.push({ idx: i, text: lines[i], level: levels[i] ?? 0 });
    }
    return out;
  }, [start, end, lines, levels]);

  return (
    <div
      ref={rootRef}
      className="log-viewport"
      style={{ height: viewportHeight }}
      onScroll={(e) => setScrollTop((e.target as HTMLDivElement).scrollTop)}
    >
      <div style={{ height: totalHeight, position: "relative" }}>
        {items.map((item) => (
          <div
            key={`${item.idx}-${item.text.slice(0, 16)}`}
            className={`log-row level-${levelName(item.level).toLowerCase()}`}
            style={{ transform: `translateY(${item.idx * ROW_HEIGHT}px)` }}
          >
            <span className="log-index">{item.idx + 1}</span>
            <span className="log-level">{levelName(item.level)}</span>
            <span className="log-text">{item.text}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

