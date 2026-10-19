import { useEffect, useState } from "react";
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
} from "recharts";
import { clearCache, enroll, fetchState, fetchConfig, saveConfig, updateAction } from "./api";
import type { Allocation, Config, Download, State, UpdateState } from "./types";
import mark from "./assets/zonn-mark.png";

type Tab = "overview" | "analytics" | "settings" | "activity";

// Sample is one polled reading kept for the utilization history chart.
export interface Sample {
  time: string;
  cpu: number;
  ram: number;
  gpu: number;
  temp: number;
}

const HISTORY_LEN = 45; // about 90s at the 2s poll interval

export default function App() {
  const [state, setState] = useState<State | null>(null);
  const [reachable, setReachable] = useState(true);
  const [history, setHistory] = useState<Sample[]>([]);
  const [tab, setTab] = useState<Tab>("overview");

  useEffect(() => {
    let active = true;
    const poll = async () => {
      try {
        const s = await fetchState();
        if (!active) return;
        setState(s);
        setReachable(true);
        setHistory((prev) => {
          const pct = (used: number, total: number) => (total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0);
          const sample: Sample = {
            time: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
            cpu: pct(s.utilization.cpu_used, s.specs.cpu_cores),
            ram: pct(s.utilization.ram_used_gb, s.specs.ram_gb),
            gpu: pct(s.utilization.gpu_mem_used_gb, s.specs.gpu_memory_gb),
            temp: Math.round(s.utilization.gpu_temp_c),
          };
          return [...prev, sample].slice(-HISTORY_LEN);
        });
      } catch {
        if (active) setReachable(false);
      }
    };
    poll();
    const id = setInterval(poll, 2000);
    return () => {
      active = false;
      clearInterval(id);
    };
  }, []);

  const titles: Record<Tab, [string, string]> = {
    overview: ["Overview", "What this machine is doing for its Zone right now."],
    analytics: ["Analytics", "Live load over the last minutes and how this machine compares."],
    settings: ["Settings", "What this machine lends, when, and to whom."],
    activity: ["Activity", "Everything the node has done, newest last."],
  };

  return (
    <div className="min-h-screen bg-colony-black text-colony-cloud">
      <Sidebar tab={tab} setTab={setTab} state={state} reachable={reachable} />
      <TopBar tab={tab} setTab={setTab} state={state} reachable={reachable} />
      <div className="md:pl-60">
        <main className="relative mx-auto max-w-5xl px-4 pb-12 pt-6 sm:px-6 md:pt-8">
          <div className="pointer-events-none absolute inset-x-0 top-0 h-60 bg-[radial-gradient(ellipse_60%_100%_at_50%_0%,rgba(26,23,32,0.8),rgba(2,3,1,0))]" aria-hidden="true" />
          <div className="relative space-y-4">
            <div className="mb-2">
              <h1 className="text-2xl font-bold tracking-tight text-colony-cloud">{titles[tab][0]}</h1>
              <p className="mt-1 text-sm text-colony-mist">{titles[tab][1]}</p>
            </div>
            {!reachable && <Banner text="Cannot reach the local node service on port 9090. Is the node running?" />}
            {reachable && !state && <Banner text="Starting up..." />}
            {state && state.update?.available ? (
              <UpdatePanel update={state.update} />
            ) : (
              state &&
              state.connection === "UPDATE_REQUIRED" && <Banner text={state.notice || "This Zone needs a newer Zonn Node. Install the latest version."} />
            )}
            {state && state.connection === "DISCONNECTED" && state.notice && <Banner text={state.notice} />}
            {state && state.waiting_for_idle && <Banner text={state.idle_note || "Waiting until you are away (Only when idle is on)."} />}
            {state && !state.enrolled && <EnrollPanel state={state} />}
            {state && tab === "overview" && <Overview state={state} />}
            {state && tab === "analytics" && <Analytics state={state} history={history} />}
            {state && tab === "settings" && <Settings state={state} />}
            {state && tab === "activity" && <Activity state={state} />}
          </div>
        </main>
      </div>
    </div>
  );
}

const TABS: { key: Tab; label: string; icon: string }[] = [
  { key: "overview", label: "Overview", icon: "M4 11l8-7 8 7v8a1 1 0 0 1-1 1h-4v-6h-6v6H5a1 1 0 0 1-1-1v-8z" },
  { key: "analytics", label: "Analytics", icon: "M3 12h4l2-6 4 12 2-6h6" },
  { key: "settings", label: "Settings", icon: "M4 6h10M18 6h2M4 12h4M12 12h8M4 18h12M20 18h0M16 4v4M10 10v4M18 16v4" },
  { key: "activity", label: "Activity", icon: "M6 3h8l4 4v14H6V3zM9 12h6M9 16h6" },
];

function NavIcon({ d }: { d: string }) {
  return (
    <svg viewBox="0 0 24 24" className="h-[18px] w-[18px]" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={d} />
    </svg>
  );
}

function Brand() {
  return (
    <div className="flex items-center gap-2.5">
      <img src={mark} alt="" width={30} height={30} className="h-[30px] w-[30px]" />
      <div>
        <div className="text-base font-bold leading-none tracking-[0.28em] text-colony-cloud">ZONN</div>
        <div className="mt-1 text-[9px] uppercase tracking-[0.22em] text-colony-grayblue">Node</div>
      </div>
    </div>
  );
}

// statusOf names the node's state in words, with a tone and whether it is live.
function statusOf(state: State | null, reachable: boolean) {
  if (!reachable) return { label: "Service not running", live: false, paused: false };
  if (!state) return { label: "Starting", live: false, paused: false };
  if (state.available === false) return { label: "Paused", live: false, paused: true };
  if (state.waiting_for_idle) return { label: "Waiting for idle", live: false, paused: true };
  if (state.status === "ONLINE") return { label: state.colony_usage?.active ? "Working for a Zone" : "Online", live: true, paused: false };
  const byConn: Record<string, string> = { CONNECTING: "Connecting", UNENROLLED: "Not enrolled", UPDATE_REQUIRED: "Update required" };
  return { label: byConn[state.connection] ?? "Offline", live: false, paused: false };
}

function StatusPill({ state, reachable }: { state: State | null; reachable: boolean }) {
  const st = statusOf(state, reachable);
  return (
    <span
      className={`inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-medium ${
        st.live ? "border-colony-midblue bg-colony-navy text-colony-ice" : "border-colony-deep bg-colony-black text-colony-mist"
      }`}
    >
      {st.paused ? (
        <PauseGlyph />
      ) : st.live ? (
        <span className="relative flex h-2 w-2" aria-hidden="true">
          <span className="absolute inline-flex h-full w-full rounded-full bg-colony-ice opacity-60 [animation:zonn-ping_1.6s_cubic-bezier(0,0,0.2,1)_infinite]" />
          <span className="relative h-2 w-2 rounded-full bg-colony-ice" />
        </span>
      ) : (
        <span className="h-2 w-2 rounded-full border border-colony-mist" aria-hidden="true" />
      )}
      {st.label}
    </span>
  );
}

function PauseGlyph() {
  return (
    <span className="inline-flex gap-[2px]" aria-hidden="true">
      <span className="h-2 w-[3px] rounded-sm bg-colony-mist" />
      <span className="h-2 w-[3px] rounded-sm bg-colony-mist" />
    </span>
  );
}

// PauseButton is the contributor's one click stop: it flips availability at
// once, the same switch as in Settings.
function PauseButton({ state, wide = false }: { state: State; wide?: boolean }) {
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const flip = async () => {
    setBusy(true);
    setFailed(false);
    try {
      const cfg = await fetchConfig();
      await saveConfig({ ...cfg, available: !cfg.available });
    } catch {
      setFailed(true);
    } finally {
      setBusy(false);
    }
  };
  const paused = state.available === false;
  return (
    <div>
      <button
        type="button"
        onClick={flip}
        disabled={busy || !state.enrolled}
        className={`inline-flex items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-semibold transition-colors disabled:opacity-50 ${wide ? "w-full" : ""} ${
          paused ? "bg-colony-ice text-colony-black hover:bg-colony-vibrant" : "border border-colony-deep text-colony-cloud hover:border-colony-midblue hover:bg-colony-navy"
        }`}
      >
        {paused ? (
          <svg viewBox="0 0 24 24" className="h-4 w-4" fill="currentColor" aria-hidden="true"><path d="M7 5l12 7-12 7V5z" /></svg>
        ) : (
          <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" aria-hidden="true"><path d="M9 6v12M15 6v12" /></svg>
        )}
        {busy ? "Applying..." : paused ? (wide ? "Resume" : "Resume contributing") : wide ? "Pause" : "Pause contributing"}
      </button>
      {failed && <p className="mt-1 text-xs text-colony-warmbeige">Could not apply that. Is the node running?</p>}
    </div>
  );
}

function Sidebar({ tab, setTab, state, reachable }: { tab: Tab; setTab: (t: Tab) => void; state: State | null; reachable: boolean }) {
  return (
    <aside className="fixed inset-y-0 left-0 z-30 hidden w-60 flex-col border-r border-colony-navy bg-colony-midnight md:flex">
      <div className="flex h-16 items-center px-5">
        <Brand />
      </div>
      <nav aria-label="Zonn Node" className="flex-1 space-y-0.5 px-3 py-2">
        {TABS.map((t) => {
          const active = tab === t.key;
          return (
            <button
              key={t.key}
              type="button"
              onClick={() => setTab(t.key)}
              aria-current={active ? "page" : undefined}
              className={`relative flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors ${
                active ? "bg-colony-navy text-colony-ice" : "text-colony-mist hover:bg-colony-navy/60 hover:text-colony-cloud"
              }`}
            >
              {active && <span className="absolute inset-y-1.5 left-0 w-0.5 rounded-full bg-colony-ice" aria-hidden="true" />}
              <NavIcon d={t.icon} />
              {t.label}
            </button>
          );
        })}
      </nav>
      <div className="space-y-3 border-t border-colony-navy p-4">
        <div className="min-w-0">
          <div className="truncate text-sm font-medium text-colony-cloud">{state?.node_name ?? "..."}</div>
          <div className="truncate font-mono text-[11px] text-colony-grayblue">{state?.coordinator_url ?? ""}</div>
        </div>
        <StatusPill state={state} reachable={reachable} />
        {state && <PauseButton state={state} wide />}
        {state?.app_version && <div className="text-[10px] text-colony-grayblue">Zonn Node {state.app_version}</div>}
      </div>
    </aside>
  );
}

function TopBar({ tab, setTab, state, reachable }: { tab: Tab; setTab: (t: Tab) => void; state: State | null; reachable: boolean }) {
  return (
    <header className="sticky top-0 z-30 border-b border-colony-navy bg-colony-black/95 backdrop-blur md:hidden">
      <div className="flex h-14 items-center justify-between px-4">
        <Brand />
        <StatusPill state={state} reachable={reachable} />
      </div>
      <nav aria-label="Zonn Node" className="flex gap-1 overflow-x-auto px-3 pb-2">
        {TABS.map((t) => (
          <button
            key={t.key}
            type="button"
            onClick={() => setTab(t.key)}
            aria-current={tab === t.key ? "page" : undefined}
            className={`flex shrink-0 items-center gap-2 rounded-lg px-3 py-1.5 text-xs ${tab === t.key ? "bg-colony-navy text-colony-ice" : "text-colony-mist"}`}
          >
            <NavIcon d={t.icon} />
            {t.label}
          </button>
        ))}
      </nav>
    </header>
  );
}

// Compute unit weights mirror the Coordinator (blueprint_v2 section 2.2): a CPU
// core is the base unit, RAM is lighter per GB, GPU memory heavier. A node's
// donation folds into this one normalized number, so a CPU heavy machine and a
// GPU heavy machine add to the same Zone pool. There is no separate CPU or GPU
// track: whatever mix you contribute converts into the same units.
const UNIT_CPU = 1.0;
const UNIT_RAM = 0.5;
const UNIT_GPU = 1.5;

function computeUnits(a: Allocation): number {
  return a.cpu_cores * UNIT_CPU + a.ram_gb * UNIT_RAM + a.gpu_memory_gb * UNIT_GPU;
}

function ContributionUnits({ alloc }: { alloc: Allocation }) {
  const total = computeUnits(alloc);
  const cpuU = alloc.cpu_cores * UNIT_CPU;
  const ramU = alloc.ram_gb * UNIT_RAM;
  const gpuU = alloc.gpu_memory_gb * UNIT_GPU;
  return (
    <div className="mt-4 rounded border border-colony-navy bg-colony-black p-3">
      <div className="flex items-baseline justify-between">
        <span className="text-sm font-medium text-colony-cloud">Your compute units</span>
        <span className="font-mono text-lg font-semibold text-colony-ice">{total.toFixed(1)}</span>
      </div>
      <p className="mt-1 text-xs text-colony-mist">
        CPU, memory, and GPU fold into one number, so it does not matter whether you lean CPU or GPU. Here that is{" "}
        {alloc.cpu_cores} cores ({cpuU.toFixed(1)}) + {alloc.ram_gb} GB RAM ({ramU.toFixed(1)}) + {alloc.gpu_memory_gb} GB GPU (
        {gpuU.toFixed(1)}). Every node adds its units to the same Zone pool.
      </p>
    </div>
  );
}

function Overview({ state }: { state: State }) {
  const u = state.utilization;
  const a = state.allocation;
  const s = state.specs;
  const hasGPU = s.gpu_memory_gb > 0;
  const pct = (used: number, total: number) => (total > 0 ? (used / total) * 100 : null);
  return (
    <div className="space-y-4">
      <section className="relative overflow-hidden rounded-xl border border-colony-midblue bg-gradient-to-br from-colony-navy via-colony-indigo to-colony-black p-5">
        <div className="flex flex-col gap-5 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate text-xl font-bold text-colony-cloud">{state.node_name}</h2>
              <StatusPill state={state} reachable />
            </div>
            <p className="mt-1.5 text-xs text-colony-mist">
              {s.os} · {s.cpu_cores} cores · {s.ram_gb} GB · {s.gpu_model || "no GPU"}
            </p>
            <p className="mt-1 text-xs text-colony-mist">
              {state.colony_id ? (
                <>
                  In Zone <span className="font-mono text-colony-cloud">{state.colony_id.slice(0, 12)}</span>
                  {state.colony_usage.active ? ", working on a request now" : ", idle"}
                </>
              ) : (
                "Not in a Zone yet: its owner adds it to one in Zonn Console."
              )}
            </p>
            <div className="mt-4 flex flex-wrap items-center gap-3">
              <PauseButton state={state} />
              <span className="text-xs text-colony-grayblue">Lending {computeUnits(a).toFixed(1)} compute units</span>
            </div>
          </div>
          <div className="grid grid-cols-3 gap-2 sm:gap-4">
            <Ring value={pct(u.cpu_used, s.cpu_cores)} label="CPU" detail={`${u.cpu_used.toFixed(1)} / ${s.cpu_cores}`} />
            <Ring value={pct(u.ram_used_gb, s.ram_gb)} label="Memory" detail={`${u.ram_used_gb.toFixed(1)} / ${s.ram_gb} GB`} />
            <Ring value={hasGPU ? pct(u.gpu_mem_used_gb, s.gpu_memory_gb) : null} label="GPU memory" detail={hasGPU ? `${u.gpu_mem_used_gb.toFixed(1)} / ${s.gpu_memory_gb} GB` : "no GPU"} />
          </div>
        </div>
      </section>

      {/* Each resource is a conservation balance: in use + contributed + free
          always adds up to the machine total, so what the machine consumes and
          what is offered to the Zone can never together exceed what exists. */}
      <Card title="Resource balance">
        <p className="text-sm text-colony-mist">
          For every resource, what your machine is using, what you contribute to the Zone, and the free room left
          always add up to your total. You can never contribute more than is free.
        </p>
        <BalanceLegend />
        <div className="mt-4 space-y-4">
          <BalanceRow label="CPU" used={u.cpu_used} contribution={a.cpu_cores} total={s.cpu_cores} unit="cores" />
          <BalanceRow label="Memory" used={u.ram_used_gb} contribution={a.ram_gb} total={s.ram_gb} unit="GB" />
          {hasGPU && (
            <BalanceRow label="GPU memory" used={u.gpu_mem_used_gb} contribution={a.gpu_memory_gb} total={s.gpu_memory_gb} unit="GB" />
          )}
        </div>
        {u.gpu_temp_c > 0 && (
          <div className="mt-4 border-t border-colony-navy pt-3">
            <Row label="GPU temperature" value={`${u.gpu_temp_c} C`} />
          </div>
        )}
      </Card>

      {state.downloads?.length > 0 && <Downloads downloads={state.downloads} />}
      <ContributionUsage state={state} />
    </div>
  );
}

// sizeText shows bytes in MB, or GB from 1 GB up.
function sizeText(n: number): string {
  return n >= 1 << 30 ? `${(n / (1 << 30)).toFixed(1)} GB` : `${Math.round(n / (1 << 20))} MB`;
}

// Downloads shows model weights being downloaded for a Zone. Each machine
// downloads only the part of a model its stage holds, once; later runs load it
// from the model cache.
function Downloads({ downloads }: { downloads: Download[] }) {
  return (
    <Card title="Downloading model weights">
      <div className="space-y-3">
        {downloads.map((d) => {
          const pct = d.total > 0 ? Math.min(100, Math.round((d.done / d.total) * 100)) : 0;
          return (
            <div key={d.model}>
              <div className="flex justify-between text-sm text-colony-cloud">
                <span>{d.model}</span>
                <span className="text-colony-mist">
                  {sizeText(d.done)} of {sizeText(d.total)} ({pct}%)
                </span>
              </div>
              <div className="mt-1 h-1.5 w-full overflow-hidden rounded bg-colony-navy">
                <div className="h-full bg-colony-ice" style={{ width: `${pct}%` }} />
              </div>
            </div>
          );
        })}
      </div>
    </Card>
  );
}

// ContributionUsage shows how much of the pledge the Zone is actually drawing
// on right now, separate from the machine's own usage. It is zero when no Zone
// task is running here, and moves when one is, so a contributor can see their
// donation being put to work rather than only what they set aside.
function ContributionUsage({ state }: { state: State }) {
  const cu = state.colony_usage;
  const a = state.allocation;
  return (
    <Card title="Zone use of your contribution">
      <p className="text-sm text-colony-mist">
        {cu.active
          ? "The Zone is running a task on your node. This is how much of what you pledged it is drawing right now."
          : "The Zone is not running anything on your node right now, so none of your pledge is in use. This moves when a task runs here."}
      </p>
      <div className="mt-3 space-y-4">
        <UsageBar label="CPU" used={cu.cpu_cores} pledged={a.cpu_cores} unit="cores" />
        <UsageBar label="Memory" used={cu.ram_gb} pledged={a.ram_gb} unit="GB" />
      </div>
    </Card>
  );
}

function UsageBar({ label, used, pledged, unit }: { label: string; used: number; pledged: number; unit: string }) {
  const pct = pledged > 0 ? Math.min(100, Math.round((used / pledged) * 100)) : 0;
  const free = Math.max(0, pledged - used);
  return (
    <div className="text-sm">
      <div className="mb-1 flex items-baseline justify-between gap-3">
        <span className="text-colony-mist">{label}</span>
        <span className="font-mono text-xs text-colony-cloud">
          {used.toFixed(1)} of {pledged} {unit} in use, {free.toFixed(1)} free
        </span>
      </div>
      <div className="h-2 w-full rounded bg-colony-navy">
        <div className="h-2 rounded bg-colony-vibrant" style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

function BalanceLegend() {
  return (
    <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-xs text-colony-mist">
      <Swatch className="bg-colony-cloud" label="In use by your machine" />
      <Swatch className="bg-colony-ice" label="Contributed to the Zone" />
      <Swatch className="bg-colony-navy" label="Free" />
    </div>
  );
}

function Swatch({ className, label }: { className: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={`h-2.5 w-2.5 rounded-sm ${className}`} />
      {label}
    </span>
  );
}

// BalanceRow renders one resource as a single full width bar split into the part
// in use, the part contributed, and the free remainder. The three always sum to
// the total. If usage plus contribution somehow exceeds the total (for example
// usage climbed after the pledge was set), the bar fills completely, free reads
// zero, and an over capacity note appears so the imbalance is never hidden.
function BalanceRow({
  label,
  used,
  contribution,
  total,
  unit,
}: {
  label: string;
  used: number;
  contribution: number;
  total: number;
  unit: string;
}) {
  const committed = used + contribution;
  const over = committed > total + 0.001;
  const free = Math.max(0, total - committed);
  const denom = over ? committed : total;
  const usedPct = denom > 0 ? (used / denom) * 100 : 0;
  const contribPct = denom > 0 ? (contribution / denom) * 100 : 0;
  const fmt = (n: number) => (Number.isInteger(n) ? String(n) : n.toFixed(1));
  return (
    <div className="text-sm">
      <div className="mb-1 flex items-baseline justify-between gap-3">
        <span className="font-medium text-colony-cloud">{label}</span>
        <span className="font-mono text-xs text-colony-mist">
          {fmt(used)} in use &middot; {contribution} contributed &middot; {fmt(free)} free &middot; {total} {unit}
        </span>
      </div>
      <div className="flex h-3 w-full overflow-hidden rounded bg-colony-navy">
        <div className="h-3 bg-colony-cloud" style={{ width: `${usedPct}%` }} />
        <div className="h-3 bg-colony-ice" style={{ width: `${contribPct}%` }} />
      </div>
      {over && (
        <div className="mt-1 flex items-center gap-1.5 text-xs text-colony-mist">
          <WarnGlyph />
          Your machine is using more than the free room left after your pledge. Lower your contribution in Settings.
        </div>
      )}
    </div>
  );
}

// Ring is a circular gauge for one live share of the machine.
function Ring({ value, label, detail }: { value: number | null; label: string; detail: string }) {
  const r = 34;
  const c = 2 * Math.PI * r;
  const v = value == null ? 0 : Math.max(0, Math.min(100, value));
  return (
    <div className="flex flex-col items-center gap-1">
      <svg viewBox="0 0 80 80" className="h-[72px] w-[72px] sm:h-[84px] sm:w-[84px]" role="img" aria-label={value == null ? `${label}: none` : `${label}: ${Math.round(v)}%`}>
        <circle cx="40" cy="40" r={r} fill="none" stroke="#1A1720" strokeWidth="7" />
        <circle cx="40" cy="40" r={r} fill="none" stroke="#F0D2A3" strokeWidth="7" strokeLinecap="round" strokeDasharray={`${(v / 100) * c} ${c}`} transform="rotate(-90 40 40)" style={{ transition: "stroke-dasharray 0.7s" }} />
        <text x="40" y="45" textAnchor="middle" fill="#C6C5BF" fontSize="15" fontWeight="700">
          {value == null ? "n/a" : `${Math.round(v)}%`}
        </text>
      </svg>
      <span className="text-[11px] font-medium text-colony-cloud">{label}</span>
      <span className="-mt-0.5 text-[10px] text-colony-grayblue">{detail}</span>
    </div>
  );
}

function WarnGlyph() {
  return (
    <span className="inline-flex h-4 w-4 items-center justify-center rounded-full bg-colony-warmbeige font-mono text-[10px] font-bold text-colony-navy">
      !
    </span>
  );
}

function Analytics({ state, history }: { state: State; history: Sample[] }) {
  const r = state.ranking;
  const u = state.utilization;
  const s = state.specs;
  const hasGPU = s.gpu_memory_gb > 0;
  const tempWarn = u.gpu_temp_c >= 90;

  const vsAverage =
    r.average_score > 0 ? Math.round(((r.contribution_score - r.average_score) / r.average_score) * 100) : 0;
  const comparison =
    r.active_nodes <= 1
      ? "You are the only active node right now."
      : vsAverage === 0
        ? "Your contribution is right at the Zone average."
        : vsAverage > 0
          ? `Your contribution is ${vsAverage}% above the Zone average.`
          : `Your contribution is ${Math.abs(vsAverage)}% below the Zone average.`;

  const peakCpu = history.reduce((m, h) => Math.max(m, h.cpu), 0);
  const peakRam = history.reduce((m, h) => Math.max(m, h.ram), 0);

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <StatCard label="Your rank" value={r.rank > 0 ? `#${r.rank}` : "--"} sub={`of ${r.active_nodes} active nodes`} accent="text-colony-ice" />
        <StatCard label="Compute units" value={r.contribution_score.toFixed(0)} sub={`Zone average ${r.average_score.toFixed(0)}`} accent="text-colony-vibrant" />
        <StatCard label="Fleet nodes" value={String(r.active_nodes)} sub="online right now" accent="text-colony-cloud" />
        {hasGPU ? (
          <StatCard
            label="GPU temperature"
            value={u.gpu_temp_c > 0 ? `${Math.round(u.gpu_temp_c)} C` : "--"}
            sub={u.gpu_temp_c > 0 ? (tempWarn ? "Running hot" : "Normal") : "no reading"}
            accent="text-colony-vibrant"
            warn={tempWarn}
          />
        ) : (
          <StatCard label="GPU" value="None" sub="not detected" accent="text-colony-mist" />
        )}
      </div>

      <Card title="Utilization over time">
        <p className="text-sm text-colony-mist">
          The share of your machine in use, sampled every two seconds. Peak so far: CPU {peakCpu}%, memory {peakRam}%.
        </p>
        <div className="mt-2">
          <ChartLegend hasGPU={hasGPU} />
        </div>
        <UtilizationChart history={history} hasGPU={hasGPU} />
      </Card>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <Card title="CPU in use">
          <Gauge used={u.cpu_used} total={s.cpu_cores} unit="cores" />
        </Card>
        <Card title="Memory in use">
          <Gauge used={u.ram_used_gb} total={s.ram_gb} unit="GB" />
        </Card>
        {hasGPU ? (
          <Card title="GPU memory in use">
            <Gauge used={u.gpu_mem_used_gb} total={s.gpu_memory_gb} unit="GB" />
          </Card>
        ) : (
          <Card title="GPU">
            <p className="text-sm text-colony-mist">No GPU detected on this machine.</p>
          </Card>
        )}
      </div>

      <Card title="Standing versus the Zone">
        <p className="mb-3 text-sm text-colony-mist">{comparison}</p>
        <ScoreBar score={r.contribution_score} average={r.average_score} />
      </Card>
    </div>
  );
}

function StatCard({
  label,
  value,
  sub,
  accent,
  warn,
}: {
  label: string;
  value: string;
  sub: string;
  accent: string;
  warn?: boolean;
}) {
  const strong = accent === "text-colony-ice";
  return (
    <section className={`min-w-0 rounded-xl border p-4 ${strong ? "border-colony-midblue bg-gradient-to-br from-colony-navy to-colony-indigo" : "border-colony-navy bg-colony-indigo"}`}>
      <div className="text-[11px] uppercase tracking-[0.14em] text-colony-mist">{label}</div>
      <div className={`mt-2 flex items-center gap-2 text-2xl font-bold tracking-tight sm:text-3xl ${strong ? "text-colony-ice" : "text-colony-cloud"}`}>
        <span>{value}</span>
        {warn && <WarnGlyph />}
      </div>
      <div className="mt-1 text-xs text-colony-grayblue">{sub}</div>
    </section>
  );
}

function ChartLegend({ hasGPU }: { hasGPU: boolean }) {
  return (
    <div className="flex flex-wrap gap-x-5 gap-y-1 text-xs text-colony-mist">
      <Swatch className="bg-colony-cloud" label="CPU" />
      <Swatch className="bg-colony-ice" label="Memory" />
      {hasGPU && <Swatch className="bg-colony-warmbeige" label="GPU memory" />}
    </div>
  );
}

function UtilizationChart({ history, hasGPU }: { history: Sample[]; hasGPU: boolean }) {
  if (history.length < 2) {
    return (
      <div className="flex h-[220px] items-center justify-center text-sm text-colony-mist">
        Collecting live samples...
      </div>
    );
  }
  return (
    <ResponsiveContainer width="100%" height={220}>
      <AreaChart data={history} margin={{ top: 8, right: 8, left: -12, bottom: 0 }}>
        <defs>
          <linearGradient id="gCpu" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#C6C5BF" stopOpacity={0.5} />
            <stop offset="100%" stopColor="#C6C5BF" stopOpacity={0.04} />
          </linearGradient>
          <linearGradient id="gRam" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#F0D2A3" stopOpacity={0.5} />
            <stop offset="100%" stopColor="#F0D2A3" stopOpacity={0.04} />
          </linearGradient>
          <linearGradient id="gGpu" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#D7CAB4" stopOpacity={0.5} />
            <stop offset="100%" stopColor="#D7CAB4" stopOpacity={0.04} />
          </linearGradient>
        </defs>
        <CartesianGrid stroke="#2F2A2D" strokeDasharray="3 3" vertical={false} />
        <XAxis dataKey="time" tick={{ fontSize: 10, fill: "#878782" }} interval="preserveStartEnd" minTickGap={44} />
        <YAxis domain={[0, 100]} tick={{ fontSize: 10, fill: "#878782" }} tickFormatter={(v) => `${v}%`} width={40} />
        <Tooltip
          contentStyle={{ background: "#0F0E12", border: "1px solid #2F2A2D", color: "#C6C5BF", borderRadius: 6, fontSize: 12 }}
          labelStyle={{ color: "#878782" }}
          formatter={(v: number, n: string) => [`${v}%`, n]}
        />
        <Area type="monotone" dataKey="cpu" name="CPU" stroke="#C6C5BF" strokeWidth={2} fill="url(#gCpu)" isAnimationActive={false} />
        <Area type="monotone" dataKey="ram" name="Memory" stroke="#F0D2A3" strokeWidth={2} fill="url(#gRam)" isAnimationActive={false} />
        {hasGPU && (
          <Area type="monotone" dataKey="gpu" name="GPU memory" stroke="#D7CAB4" strokeWidth={2} fill="url(#gGpu)" isAnimationActive={false} />
        )}
      </AreaChart>
    </ResponsiveContainer>
  );
}

function Gauge({ used, total, unit }: { used: number; total: number; unit: string }) {
  return (
    <div className="flex justify-center py-2">
      <Ring value={total > 0 ? (used / total) * 100 : null} label={`${used.toFixed(1)} of ${total} ${unit}`} detail="of the whole machine" />
    </div>
  );
}

function ScoreBar({ score, average }: { score: number; average: number }) {
  const max = Math.max(score, average, 1);
  return (
    <div className="space-y-3 pt-2">
      <LabeledBar label="You" value={score} max={max} color="bg-colony-ice" />
      <LabeledBar label="Average" value={average} max={max} color="bg-colony-grayblue" />
    </div>
  );
}

function LabeledBar({ label, value, max, color }: { label: string; value: number; max: number; color: string }) {
  const pct = max > 0 ? Math.min(100, Math.round((value / max) * 100)) : 0;
  return (
    <div className="text-sm">
      <div className="mb-1 flex justify-between">
        <span className="text-colony-mist">{label}</span>
        <span className="font-mono text-xs text-colony-cloud">{value.toFixed(0)}</span>
      </div>
      <div className="h-2 w-full rounded bg-colony-navy">
        <div className={`h-2 rounded ${color}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

function Settings({ state }: { state: State }) {
  const [cfg, setCfg] = useState<Config | null>(null);
  const [loadError, setLoadError] = useState(false);
  const [savingAvail, setSavingAvail] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    fetchConfig()
      .then((c) => active && setCfg(c))
      .catch(() => active && setLoadError(true));
    return () => {
      active = false;
    };
  }, []);

  if (loadError) {
    return <Banner text="Could not load the node settings from the local service on port 9090." />;
  }
  if (!cfg) {
    return <Banner text="Loading settings..." />;
  }

  // Free capacity is the total minus what the machine is using right now, so a
  // pledge can never exceed what is actually available. Floored to whole units
  // to keep the sliders on integer steps and guarantee usage + pledge <= total.
  const u = state.utilization;
  const freeCPU = Math.max(0, Math.floor(state.specs.cpu_cores - u.cpu_used));
  const freeRAM = Math.max(0, Math.floor(state.specs.ram_gb - u.ram_used_gb));
  const freeGPU = Math.max(0, Math.floor(state.specs.gpu_memory_gb - u.gpu_mem_used_gb));

  const editAlloc = (key: keyof Allocation, value: number) => {
    setCfg({ ...cfg, allocation: { ...cfg.allocation, [key]: value } });
    setSaved(false);
  };

  // Availability is the make-me-available switch and applies immediately, so a
  // contributor can pause without hunting for a Save button.
  const toggleAvailable = async () => {
    const next = { ...cfg, available: !cfg.available };
    setCfg(next);
    setSavingAvail(true);
    setError("");
    try {
      setCfg(await saveConfig(next));
    } catch {
      setCfg({ ...next, available: !next.available });
      setError("Could not apply that change. Is the node running?");
    } finally {
      setSavingAvail(false);
    }
  };

  const save = async () => {
    setSaving(true);
    setError("");
    try {
      setCfg(await saveConfig(cfg));
      setSaved(true);
    } catch {
      setError("Could not save. Is the node running?");
    } finally {
      setSaving(false);
    }
  };

  const toggleShare = async () => {
    const next = { ...cfg, share_community: !cfg.share_community };
    setCfg(next);
    try {
      setCfg(await saveConfig(next));
    } catch {
      setCfg({ ...next, share_community: !next.share_community });
      setError("Could not apply that change. Is the node running?");
    }
  };

  // applyNow saves one setting at once, rolling back on failure.
  const applyNow = async (next: Config) => {
    const before = cfg;
    setCfg(next);
    try {
      setCfg(await saveConfig(next));
    } catch {
      setCfg(before);
      setError("Could not apply that change. Is the node running?");
    }
  };

  const toggleCrashReports = async () => {
    const next = { ...cfg, crash_reports_enabled: !cfg.crash_reports_enabled };
    setCfg(next);
    try {
      setCfg(await saveConfig(next));
    } catch {
      setCfg({ ...next, crash_reports_enabled: !next.crash_reports_enabled });
      setError("Could not apply that change. Is the node running?");
    }
  };

  return (
    <div className="space-y-4">
      <Card title="Availability">
        <div className="flex items-center justify-between gap-4">
          <div>
            <div className="text-sm font-medium text-colony-cloud">
              {cfg.available ? "This machine is available to the Zone" : "This machine is paused"}
            </div>
            <p className="mt-1 max-w-md text-sm text-colony-mist">
              {cfg.available
                ? "The node is registered and contributing the resources you set below. Turn this off to stop contributing without quitting the app."
                : "The node has stopped contributing and shows as offline to the Zone. Turn this on to make yourself available again."}
            </p>
          </div>
          <Toggle on={cfg.available} busy={savingAvail} onChange={toggleAvailable} label="Available to the Zone" />
        </div>
      </Card>

      <Card title="Only when idle">
        <div className="flex items-center justify-between gap-4">
          <div>
            <div className="text-sm font-medium text-colony-cloud">
              {cfg.only_when_idle ? `Contributing only after you have been away for ${cfg.idle_minutes} minutes` : "Contributing whenever this machine is available"}
            </div>
            <p className="mt-1 max-w-md text-sm text-colony-mist">
              When on, this machine contributes only while nobody is using its keyboard or mouse, and stops as soon as
              you come back. A Zone using it moves its work to its other machines.
            </p>
            {cfg.only_when_idle && (
              <label className="mt-2 flex items-center gap-2 text-sm text-colony-mist">
                Away for at least
                <select
                  value={cfg.idle_minutes}
                  onChange={(e) => applyNow({ ...cfg, idle_minutes: Number(e.target.value) })}
                  className="rounded border border-colony-navy bg-colony-black px-2 py-1 text-sm text-colony-cloud outline-none focus:border-colony-midblue"
                >
                  {[1, 2, 5, 10, 15, 30, 60].map((m) => (
                    <option key={m} value={m}>
                      {m} {m === 1 ? "minute" : "minutes"}
                    </option>
                  ))}
                </select>
              </label>
            )}
          </div>
          <Toggle
            on={cfg.only_when_idle}
            busy={false}
            onChange={() => applyNow({ ...cfg, only_when_idle: !cfg.only_when_idle })}
            label="Only when idle"
          />
        </div>
      </Card>

      {state.desktop && (
        <Card title="Start when you sign in">
          <div className="flex items-center justify-between gap-4">
            <div>
              <div className="text-sm font-medium text-colony-cloud">
                {cfg.auto_start ? "Zonn Node starts in the background when you sign in" : "Zonn Node starts only when you open it"}
              </div>
              <p className="mt-1 max-w-md text-sm text-colony-mist">
                Closing the window keeps Zonn Node running; open it again from the tray icon (on macOS, from the Dock and
                the Zonn Node menu). Quit it from there to stop contributing.
              </p>
            </div>
            <Toggle
              on={cfg.auto_start}
              busy={false}
              onChange={() => applyNow({ ...cfg, auto_start: !cfg.auto_start })}
              label="Start when you sign in"
            />
          </div>
        </Card>
      )}

      <ModelCache state={state} cfg={cfg} apply={applyNow} />

      <Card title="Community sharing">
        <div className="flex items-center justify-between gap-4">
          <div>
            <div className="text-sm font-medium text-colony-cloud">
              {cfg.share_community ? "Shared with the community pool" : "Only your own Zones use this machine"}
            </div>
            <p className="mt-1 max-w-md text-sm text-colony-mist">
              When on, Zonn may also include this machine in community Zones that serve other people, once it has been online
              long enough. It then holds middle layers and receives only hidden states (the numbers passed between layers),
              not the prompts and answers themselves: the first and last stages run on machines Zonn operates. Now and then a trusted machine
              repeats some of its work to check it; repeated mismatches take it out of community Zones. Turning this off takes
              the machine out of any community Zone at once.
            </p>
            {state.community?.status && (
              <p className="mt-2 max-w-md text-sm text-colony-cloud">
                <span className={`mr-2 inline-block rounded border px-1.5 text-[10px] font-semibold uppercase tracking-wide ${state.community.eligible ? "border-colony-midblue text-colony-ice" : "border-colony-deep text-colony-mist"}`}>
                  {state.community.eligible ? "Eligible" : "Status"}
                </span>
                {state.community.status}
              </p>
            )}
          </div>
          <Toggle on={cfg.share_community} busy={false} onChange={toggleShare} label="Share with the community pool" />
        </div>
      </Card>

      <Card title="Crash reports">
        <div className="flex items-center justify-between gap-4">
          <div>
            <div className="text-sm font-medium text-colony-cloud">
              {cfg.crash_reports_enabled ? "Anonymized crash reports are on" : "Crash reports are off"}
            </div>
            <p className="mt-1 max-w-md text-sm text-colony-mist">
              If the node recovers from a crash, it sends the error type, a stack trace, and your operating system to
              the Coordinator so it shows up in Zonn Console. Never your hostname, username, IP address, or files.
            </p>
          </div>
          <Toggle on={cfg.crash_reports_enabled} busy={false} onChange={toggleCrashReports} label="Send anonymized crash reports" />
        </div>
      </Card>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Card title="Identity and connection">
          <Field label="Node name">
            <input
              type="text"
              value={cfg.node_name}
              onChange={(e) => {
                setCfg({ ...cfg, node_name: e.target.value });
                setSaved(false);
              }}
              className="w-full rounded border border-colony-navy bg-colony-black px-3 py-2 text-sm text-colony-cloud outline-none focus:border-colony-midblue"
            />
          </Field>
          <Field label="Coordinator address">
            <input
              type="text"
              value={cfg.coordinator_url}
              spellCheck={false}
              onChange={(e) => {
                setCfg({ ...cfg, coordinator_url: e.target.value });
                setSaved(false);
              }}
              className="w-full rounded border border-colony-navy bg-colony-black px-3 py-2 font-mono text-sm text-colony-cloud outline-none focus:border-colony-midblue"
            />
          </Field>
          <div className="pt-1">
            <Row label="Node identity" value={state.enrolled ? cfg.node_id : "Not enrolled yet"} mono />
            <Row label="App version" value={state.app_version} mono />
          </div>
          <p className="text-xs text-colony-mist">
            Changing the Coordinator address means enrolling this machine again for the new Coordinator; its
            credential is only ever sent to the Coordinator that issued it.
          </p>
        </Card>

        <Card title="Detected hardware">
          <Row label="Operating system" value={state.specs.os} />
          <Row label="Architecture" value={state.specs.arch} />
          <Row label="CPU cores" value={String(state.specs.cpu_cores)} />
          <Row label="Memory" value={`${state.specs.ram_gb} GB`} />
          <Row label="Disk" value={`${state.specs.disk_gb} GB`} />
          <Row label="GPU" value={state.specs.gpu_model || "None detected"} />
          {state.specs.gpu_memory_gb > 0 && <Row label="GPU memory" value={`${state.specs.gpu_memory_gb} GB`} />}
        </Card>
      </div>

      <Card title="What you contribute">
        <p className="mb-3 text-sm text-colony-mist">
          Each slider stops at what is free right now, your total minus what your machine is already using, so your
          contribution and your own usage always stay in balance. Zero means you contribute none of that resource.
        </p>
        <Slider
          label="CPU cores"
          value={cfg.allocation.cpu_cores}
          max={freeCPU}
          unit="cores"
          note={`${freeCPU} of ${state.specs.cpu_cores} free now`}
          onChange={(v) => editAlloc("cpu_cores", v)}
        />
        <Slider
          label="Memory"
          value={cfg.allocation.ram_gb}
          max={freeRAM}
          unit="GB"
          note={`${freeRAM} of ${state.specs.ram_gb} free now`}
          onChange={(v) => editAlloc("ram_gb", v)}
        />
        {state.specs.gpu_memory_gb > 0 && (
          <Slider
            label="GPU memory"
            value={cfg.allocation.gpu_memory_gb}
            max={freeGPU}
            unit="GB"
            note={`${freeGPU} of ${state.specs.gpu_memory_gb} free now`}
            onChange={(v) => editAlloc("gpu_memory_gb", v)}
          />
        )}
        <Slider label="Bandwidth" value={cfg.allocation.bandwidth_mbps} max={1000} unit="Mbps" onChange={(v) => editAlloc("bandwidth_mbps", v)} />
        <ContributionUnits alloc={cfg.allocation} />
        <div className="mt-4 flex items-center gap-3">
          <button
            onClick={save}
            disabled={saving}
            className="rounded bg-colony-ice px-4 py-2 text-sm font-medium text-colony-black hover:bg-colony-vibrant disabled:opacity-60"
          >
            {saving ? "Saving..." : "Save settings"}
          </button>
          {saved && <span className="text-sm text-colony-mist">Saved and applied.</span>}
          {error && <span className="text-sm text-colony-mist">{error}</span>}
        </div>
      </Card>
    </div>
  );
}

// ModelCache shows the downloaded model weights and their limit. Past the
// limit, the models used least recently are removed, never one a Zone is using
// here; Clear removes everything not in use.
function ModelCache({ state, cfg, apply }: { state: State; cfg: Config; apply: (next: Config) => Promise<void> }) {
  const cache = state.model_cache;
  const [clearing, setClearing] = useState(false);
  const [error, setError] = useState("");
  const capBytes = cfg.model_cache_gb * (1 << 30);
  const pct = capBytes > 0 ? Math.min(100, Math.round((cache.bytes / capBytes) * 100)) : 0;
  const clear = async () => {
    setClearing(true);
    setError("");
    try {
      await clearCache();
    } catch {
      setError("Could not clear the cache. Is the node running?");
    } finally {
      setClearing(false);
    }
  };
  return (
    <Card
      title="Model cache"
      action={
        <button
          type="button"
          onClick={clear}
          disabled={clearing || !cache.models?.some((m) => !m.in_use)}
          className="rounded border border-colony-midblue px-2 py-1 text-xs text-colony-ice hover:bg-colony-ice hover:text-colony-black disabled:opacity-50"
        >
          {clearing ? "Clearing" : "Clear cache"}
        </button>
      }
    >
      <p className="text-sm text-colony-mist">
        Model weights this machine downloaded for Zones, kept so the next run loads without downloading again. Past the
        limit, the models used least recently are removed; a model a Zone is using here is never removed.
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-3 text-sm text-colony-cloud">
        <span>
          {sizeText(cache.bytes)} used{capBytes > 0 ? ` of ${cfg.model_cache_gb} GB (${pct}%)` : ", no limit"}
        </span>
        <label className="flex items-center gap-2 text-colony-mist">
          Limit
          <select
            value={cfg.model_cache_gb}
            onChange={(e) => apply({ ...cfg, model_cache_gb: Number(e.target.value) })}
            className="rounded border border-colony-navy bg-colony-black px-2 py-1 text-sm text-colony-cloud outline-none focus:border-colony-midblue"
          >
            {[...new Set([1, 2, 5, 10, 20, 50, 100, cfg.model_cache_gb])]
              .filter((g) => g > 0)
              .sort((a, b) => a - b)
              .map((g) => (
                <option key={g} value={g}>
                  {g} GB
                </option>
              ))}
            <option value={0}>No limit</option>
          </select>
        </label>
      </div>
      {capBytes > 0 && (
        <div className="mt-2 h-1.5 w-full overflow-hidden rounded bg-colony-navy">
          <div className="h-full bg-colony-ice" style={{ width: `${pct}%` }} />
        </div>
      )}
      {cache.models?.length > 0 ? (
        <ul className="mt-3 space-y-1 text-sm">
          {[...cache.models].reverse().map((m) => (
            <li key={m.name} className="flex justify-between gap-3">
              <span className="text-colony-cloud">
                {m.name}
                {m.in_use && <span className="ml-2 text-xs font-medium text-colony-ice">In use</span>}
              </span>
              <span className="shrink-0 text-colony-mist">
                {sizeText(m.bytes)}, used {new Date(m.last_used).toLocaleString()}
              </span>
            </li>
          ))}
        </ul>
      ) : (
        <p className="mt-3 text-sm text-colony-mist">No models downloaded yet.</p>
      )}
      <p className="mt-2 font-mono text-xs text-colony-mist">{cache.dir}</p>
      {error && <p className="mt-2 text-sm text-colony-warmbeige">{error}</p>}
    </Card>
  );
}

function Toggle({ on, busy, onChange, label }: { on: boolean; busy: boolean; onChange: () => void; label: string }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={busy}
      onClick={onChange}
      className={`relative inline-flex h-7 w-12 shrink-0 items-center rounded-full transition-colors disabled:opacity-60 ${
        on ? "bg-colony-ice" : "border border-colony-deep bg-colony-black"
      }`}
    >
      <span
        className={`inline-block h-5 w-5 transform rounded-full transition-transform ${on ? "bg-colony-black" : "bg-colony-grayblue"} ${
          on ? "translate-x-6" : "translate-x-1"
        }`}
      />
    </button>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block text-sm">
      <span className="mb-1 block text-colony-mist">{label}</span>
      {children}
    </label>
  );
}

function Activity({ state }: { state: State }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    const text = state.events.map((e) => `${e.time} ${e.level} ${e.message}`).join("\n");
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  };

  return (
    <Card
      title="Activity"
      action={
        <button onClick={copy} className="rounded border border-colony-navy px-2 py-1 text-xs text-colony-mist hover:text-colony-cloud">
          {copied ? "Copied" : "Copy log"}
        </button>
      }
    >
      <div className="max-h-[28rem] overflow-y-auto font-mono text-xs">
        {state.events.length === 0 && <div className="text-colony-mist">No activity yet.</div>}
        {state.events.map((e, i) => (
          <div key={i} className="flex gap-3 border-b border-colony-navy py-1.5 last:border-0">
            <span className="shrink-0 text-colony-mist">{new Date(e.time).toLocaleTimeString()}</span>
            <LevelTag level={e.level} />
            <span className="text-colony-cloud">{e.message}</span>
          </div>
        ))}
      </div>
    </Card>
  );
}

function LevelTag({ level }: { level: string }) {
  const map: Record<string, string> = {
    INFO: "border border-colony-deep text-colony-grayblue",
    WARN: "border border-colony-midblue text-colony-warmbeige",
    ERROR: "bg-colony-warmbeige text-colony-black",
  };
  const cls = map[level] ?? "bg-colony-deep text-colony-grayblue";
  return <span className={`shrink-0 rounded px-1.5 ${cls}`}>{level}</span>;
}

function Card({ title, action, children }: { title: string; action?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="min-w-0 rounded-xl border border-colony-navy bg-colony-indigo p-4 sm:p-5">
      <div className="mb-3 flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold text-colony-cloud">{title}</h2>
        {action}
      </div>
      <div className="space-y-2">{children}</div>
    </section>
  );
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-4 text-sm">
      <span className="text-colony-mist">{label}</span>
      <span className={`text-right text-colony-cloud ${mono ? "font-mono text-xs" : ""}`}>{value}</span>
    </div>
  );
}

function Slider({
  label,
  value,
  max,
  unit,
  note,
  onChange,
}: {
  label: string;
  value: number;
  max: number;
  unit: string;
  note?: string;
  onChange: (v: number) => void;
}) {
  const safeMax = Math.max(0, max);
  return (
    <div className="text-sm">
      <div className="mb-1 flex items-baseline justify-between gap-3">
        <span className="text-colony-mist">
          {label}
          {note && <span className="ml-2 text-xs text-colony-mist">({note})</span>}
        </span>
        <span className="font-mono text-xs text-colony-cloud">
          {value} / {safeMax} {unit}
        </span>
      </div>
      <input
        type="range"
        min={0}
        max={safeMax}
        value={Math.min(value, safeMax)}
        onChange={(e) => onChange(Number(e.target.value))}
        disabled={safeMax === 0}
      />
    </div>
  );
}

// EnrollPanel connects a new machine to its owner's account. The owner creates
// a one time code in Zonn Console (Nodes, Add a machine) and pastes it here.
function EnrollPanel({ state }: { state: State }) {
  const [address, setAddress] = useState(state.coordinator_url);
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      if (address.trim() && address.trim() !== state.coordinator_url) {
        const cfg = await fetchConfig();
        await saveConfig({ ...cfg, coordinator_url: address.trim() });
      }
      await enroll(code.trim());
      setCode("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Enrollment failed");
    } finally {
      setBusy(false);
    }
  };

  const field =
    "w-full rounded border border-colony-navy bg-colony-black px-3 py-2 font-mono text-sm text-colony-cloud outline-none focus:border-colony-midblue";
  return (
    <div className="mb-6">
      <Card title="Enroll this machine">
        <p className="text-sm text-colony-mist">
          Before this machine can join a Zone, connect it to your Zonn account. In Zonn Console, open Nodes, choose
          Add a machine, and paste the code here. Codes work once and expire after an hour.
        </p>
        <form onSubmit={submit} className="space-y-3 pt-2">
          <Field label="Coordinator address">
            <input className={field} value={address} spellCheck={false} onChange={(e) => setAddress(e.target.value)} />
          </Field>
          <Field label="Enrollment code">
            <input
              className={field}
              value={code}
              spellCheck={false}
              placeholder="ze_..."
              onChange={(e) => setCode(e.target.value)}
            />
          </Field>
          {error && <p className="text-sm text-colony-warmbeige">{error}</p>}
          <button
            type="submit"
            disabled={busy || !code.trim()}
            className="rounded bg-colony-ice px-4 py-2 text-sm font-medium text-colony-black hover:bg-colony-vibrant disabled:opacity-60"
          >
            {busy ? "Enrolling..." : "Enroll this machine"}
          </button>
        </form>
      </Card>
    </div>
  );
}

// UpdatePanel offers a newer Zonn Node: one button downloads it from the
// backend, checks its SHA-256, and opens the installer.
function UpdatePanel({ update }: { update: UpdateState }) {
  const [error, setError] = useState("");
  const mb = (n: number) => (n / (1 << 20)).toFixed(n < 100 << 20 ? 1 : 0);
  const install = async () => {
    setError("");
    try {
      await updateAction("install");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not start the update.");
    }
  };
  const pct = update.size > 0 ? Math.min(100, Math.round((update.done / update.size) * 100)) : 0;
  return (
    <div className="mb-4 rounded-md border border-colony-navy bg-colony-navy px-4 py-3 text-sm text-colony-cloud">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="font-medium">
            Zonn Node {update.version} is available{update.required ? ", and this Zone requires it" : ""}
          </div>
          <div className="mt-0.5 text-xs text-colony-mist">
            {update.status === "downloading"
              ? `Downloading: ${mb(update.done)} of ${mb(update.size)} MB (${pct}%)`
              : update.status === "opened"
                ? "Checked against its SHA-256 and opened. Finish the installer, then open Zonn Node again."
                : `${mb(update.size)} MB, downloaded from your Zone's backend and checked against its SHA-256 before it opens.`}
          </div>
          {(update.error || error) && <div className="mt-1 text-xs text-colony-warmbeige">{update.error || error}</div>}
        </div>
        {update.status !== "downloading" && update.status !== "opened" && (
          <button
            type="button"
            onClick={install}
            className="rounded bg-colony-ice px-3 py-1.5 text-sm font-medium text-colony-black hover:bg-colony-vibrant"
          >
            {update.status === "error" ? "Try again" : "Download and install"}
          </button>
        )}
      </div>
      {update.status === "downloading" && (
        <div className="mt-2 h-1.5 w-full overflow-hidden rounded bg-colony-navy">
          <div className="h-full bg-colony-ice" style={{ width: `${pct}%` }} />
        </div>
      )}
    </div>
  );
}

function Banner({ text }: { text: string }) {
  return (
    <div role="status" className="flex items-start gap-3 rounded-xl border border-colony-midblue bg-colony-navy px-4 py-3 text-sm text-colony-warmbeige">
      <svg viewBox="0 0 24 24" className="mt-0.5 h-4 w-4 shrink-0" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
        <path d="M12 8v5M12 16.5v.5" />
        <circle cx="12" cy="12" r="9" />
      </svg>
      <span>{text}</span>
    </div>
  );
}
