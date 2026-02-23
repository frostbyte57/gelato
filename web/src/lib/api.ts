import type { APIErrorEnvelope, CreateListenerRequest, FiltersDTO, Snapshot } from "../types/api";

async function parse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let message = res.statusText;
    try {
      const data = (await res.json()) as APIErrorEnvelope;
      message = data.error?.message ?? message;
    } catch {
      // ignore
    }
    throw new Error(message);
  }
  return (await res.json()) as T;
}

export async function fetchSnapshot(): Promise<Snapshot> {
  return parse<Snapshot>(await fetch("/api/v1/snapshot"));
}

export async function getFilters(): Promise<FiltersDTO> {
  return parse<FiltersDTO>(await fetch("/api/v1/filters"));
}

export async function patchFilters(body: FiltersDTO): Promise<FiltersDTO> {
  return parse<FiltersDTO>(
    await fetch("/api/v1/filters", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
}

export async function createListener(body: CreateListenerRequest) {
  return parse(await fetch("/api/v1/listeners", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }));
}

export async function deleteListener(id: string) {
  return parse(await fetch(`/api/v1/listeners/${encodeURIComponent(id)}`, { method: "DELETE" }));
}

export async function retryListener(id: string) {
  return parse(await fetch(`/api/v1/listeners/${encodeURIComponent(id)}/retry`, { method: "POST" }));
}

export async function toggleSourceMute(key: string) {
  return parse(await fetch(`/api/v1/sources/${encodeURIComponent(key)}/toggle-mute`, { method: "POST" }));
}

export async function clearSource(key: string) {
  return parse(await fetch(`/api/v1/sources/${encodeURIComponent(key)}/clear`, { method: "POST" }));
}

