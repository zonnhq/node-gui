import { useEffect, useState } from "react";
import {
  RadialBar,
  RadialBarChart,
  PolarAngleAxis,
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
} from "recharts";
import { enroll, fetchState, fetchConfig, saveConfig } from "./api";
import type { Allocation, Config, State } from "./types";

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

  return (
    <div className="min-h-screen bg-colony-cloud text-colony-charcoal">
      <Header state={state} reachable={reachable} />
      <Tabs tab={tab} setTab={setTab} />
      <main className="mx-auto max-w-5xl px-6 py-6">
        {!reachable && <Banner text="Cannot reach the local node service on port 9090. Is the node running?" />}
        {reachable && !state && <Banner text="Starting up..." />}
        {state && state.connection === "UPDATE_REQUIRED" && (
          <div className="mb-4">
            <Banner text={state.notice || "This Zone needs a newer Zonn Node. Install the latest version."} />
          </div>
        )}
        {state && state.waiting_for_idle && (
          <div className="mb-4">
            <Banner text={state.idle_note || "Waiting until you are away (Only when idle is on)."} />
          </div>
        )}
        {state && !state.enrolled && <EnrollPanel state={state} />}
        {state && tab === "overview" && <Overview state={state} />}
        {state && tab === "analytics" && <Analytics state={state} history={history} />}
        {state && tab === "settings" && <Settings state={state} />}
        {state && tab === "activity" && <Activity state={state} />}
      </main>
    </div>
  );
}

function Header({ state, reachable }: { state: State | null; reachable: boolean }) {
  const online = reachable && state?.status === "ONLINE";
  const paused = reachable && state?.available === false;
  return (
    <header className="bg-colony-navy px-6 py-4">
      <div className="mx-auto flex max-w-5xl items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="h-8 w-8 rounded-md bg-colony-core" />
          <div>
            <div className="text-lg font-semibold text-colony-cloud">Zonn</div>
            <div className="text-xs text-colony-ice">One Zone. Infinite Compute.</div>
          </div>
        </div>
        <div className="flex items-center gap-4">
          <div className="text-right">
            <div className="text-sm font-medium text-colony-cloud">{state?.node_name ?? "..."}</div>
            <div className="font-mono text-xs text-colony-mist">{state?.coordinator_url ?? ""}</div>
          </div>
          <StatusPill
            online={online}
            paused={paused}
            waiting={reachable && !!state?.waiting_for_idle}
            connection={state?.connection ?? "DISCONNECTED"}
          />
        </div>
      </div>
    </header>
  );
}

function StatusPill({
  online,
  paused,
  waiting,
  connection,
}: {
  online: boolean;
  paused: boolean;
  waiting: boolean;
  connection: string;
}) {
  // The palette carries no red or green, so status is icon plus label plus the
  // active/offline tones from the Drapery Drama palette (see blueprint.md 8.1),
  // never color alone.
  const label = paused
    ? "Paused"
    : waiting
      ? "Waiting for idle"
    : online
      ? "Online"
      : connection === "CONNECTING"
        ? "Connecting"
        : connection === "UNENROLLED"
          ? "Not enrolled"
          : connection === "UPDATE_REQUIRED"
            ? "Update required"
            : "Offline";
  const cls = online ? "bg-colony-ice text-colony-midnight" : "bg-colony-mist text-colony-navy";
  const dot = online ? "bg-colony-deep" : "bg-colony-slate";
  return (
    <span className={`inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-medium ${cls}`}>
      {paused || waiting ? <PauseGlyph /> : <span className={`h-2 w-2 rounded-full ${dot}`} />}
      {label}
    </span>
  );
}

function PauseGlyph() {
  return (
    <span className="inline-flex gap-[2px]">
      <span className="h-2 w-[3px] rounded-sm bg-colony-slate" />
      <span className="h-2 w-[3px] rounded-sm bg-colony-slate" />
    </span>
  );
}

function Tabs({ tab, setTab }: { tab: Tab; setTab: (t: Tab) => void }) {
  const items: { key: Tab; label: string }[] = [
    { key: "overview", label: "Overview" },
    { key: "analytics", label: "Analytics" },
    { key: "settings", label: "Settings" },
    { key: "activity", label: "Activity" },
  ];
  return (
    <nav className="border-b border-colony-mist bg-colony-nearwhite px-6">
      <div className="mx-auto flex max-w-5xl gap-1">
        {items.map((it) => (
          <button
            key={it.key}
            onClick={() => setTab(it.key)}
            className={`border-b-2 px-4 py-3 text-sm font-medium ${
              tab === it.key
                ? "border-colony-core text-colony-core"
                : "border-transparent text-colony-slate hover:text-colony-charcoal"
            }`}
          >
            {it.label}
          </button>
        ))}
      </div>
    </nav>
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
    <div className="mt-4 rounded border border-colony-mist bg-colony-lightblue p-3">
      <div className="flex items-baseline justify-between">
        <span className="text-sm font-medium text-colony-charcoal">Your compute units</span>
        <span className="font-mono text-lg font-semibold text-colony-core">{total.toFixed(1)}</span>
      </div>
      <p className="mt-1 text-xs text-colony-slate">
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
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Card title="Node">
          <Row label="Name" value={state.node_name} />
          <Row label="Identity" value={state.node_id} mono />
          <Row label="Connection" value={state.connection} />
          <Row label="Zone" value={state.colony_id || "Not in a Zone"} mono={!!state.colony_id} />
        </Card>
        <Card title="Machine">
          <Row label="System" value={s.os} />
          <Row label="CPU" value={`${s.cpu_cores} cores`} />
          <Row label="Memory" value={`${s.ram_gb} GB`} />
          <Row label="GPU" value={s.gpu_model || "None detected"} />
        </Card>
      </div>

      {/* Each resource is a conservation balance: in use + contributed + free
          always adds up to the machine total, so what the machine consumes and
          what is offered to the Zone can never together exceed what exists. */}
      <Card title="Resource balance">
        <p className="text-sm text-colony-slate">
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
          <div className="mt-4 border-t border-colony-mist pt-3">
            <Row label="GPU temperature" value={`${u.gpu_temp_c} C`} />
          </div>
        )}
      </Card>

      <ContributionUsage state={state} />
    </div>
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
      <p className="text-sm text-colony-slate">
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
        <span className="text-colony-slate">{label}</span>
        <span className="font-mono text-xs text-colony-charcoal">
          {used.toFixed(1)} of {pledged} {unit} in use, {free.toFixed(1)} free
        </span>
      </div>
      <div className="h-2 w-full rounded bg-colony-mist">
        <div className="h-2 rounded bg-colony-vibrant" style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

function BalanceLegend() {
  return (
    <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-xs text-colony-slate">
      <Swatch className="bg-colony-core" label="In use by your machine" />
      <Swatch className="bg-colony-ice" label="Contributed to the Zone" />
      <Swatch className="bg-colony-mist" label="Free" />
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
        <span className="font-medium text-colony-charcoal">{label}</span>
        <span className="font-mono text-xs text-colony-slate">
          {fmt(used)} in use &middot; {contribution} contributed &middot; {fmt(free)} free &middot; {total} {unit}
        </span>
      </div>
      <div className="flex h-3 w-full overflow-hidden rounded bg-colony-mist">
        <div className="h-3 bg-colony-core" style={{ width: `${usedPct}%` }} />
        <div className="h-3 bg-colony-ice" style={{ width: `${contribPct}%` }} />
      </div>
      {over && (
        <div className="mt-1 flex items-center gap-1.5 text-xs text-colony-slate">
          <WarnGlyph />
          Your machine is using more than the free room left after your pledge. Lower your contribution in Settings.
        </div>
      )}
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
        <StatCard label="Your rank" value={r.rank > 0 ? `#${r.rank}` : "--"} sub={`of ${r.active_nodes} active nodes`} accent="text-colony-core" />
        <StatCard label="Compute units" value={r.contribution_score.toFixed(0)} sub={`Zone average ${r.average_score.toFixed(0)}`} accent="text-colony-deep" />
        <StatCard label="Fleet nodes" value={String(r.active_nodes)} sub="online right now" accent="text-colony-core" />
        {hasGPU ? (
          <StatCard
            label="GPU temperature"
            value={u.gpu_temp_c > 0 ? `${Math.round(u.gpu_temp_c)} C` : "--"}
            sub={u.gpu_temp_c > 0 ? (tempWarn ? "Running hot" : "Normal") : "no reading"}
            accent="text-colony-deep"
            warn={tempWarn}
          />
        ) : (
          <StatCard label="GPU" value="None" sub="not detected" accent="text-colony-slate" />
        )}
      </div>

      <Card title="Utilization over time">
        <p className="text-sm text-colony-slate">
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
            <p className="text-sm text-colony-slate">No GPU detected on this machine.</p>
          </Card>
        )}
      </div>

      <Card title="Standing versus the Zone">
        <p className="mb-3 text-sm text-colony-slate">{comparison}</p>
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
  return (
    <section className="rounded-md border border-colony-mist bg-colony-nearwhite p-4">
      <div className="text-xs font-semibold uppercase tracking-wide text-colony-slate">{label}</div>
      <div className={`mt-1 flex items-center gap-2 text-3xl font-semibold ${accent}`}>
        <span>{value}</span>
        {warn && <WarnGlyph />}
      </div>
      <div className="mt-1 text-xs text-colony-slate">{sub}</div>
    </section>
  );
}

function ChartLegend({ hasGPU }: { hasGPU: boolean }) {
  return (
    <div className="flex flex-wrap gap-x-5 gap-y-1 text-xs text-colony-slate">
      <Swatch className="bg-colony-core" label="CPU" />
      <Swatch className="bg-colony-ice" label="Memory" />
      {hasGPU && <Swatch className="bg-colony-vibrant" label="GPU memory" />}
    </div>
  );
}

function UtilizationChart({ history, hasGPU }: { history: Sample[]; hasGPU: boolean }) {
  if (history.length < 2) {
    return (
      <div className="flex h-[220px] items-center justify-center text-sm text-colony-slate">
        Collecting live samples...
      </div>
    );
  }
  return (
    <ResponsiveContainer width="100%" height={220}>
      <AreaChart data={history} margin={{ top: 8, right: 8, left: -12, bottom: 0 }}>
        <defs>
          <linearGradient id="gCpu" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#544943" stopOpacity={0.5} />
            <stop offset="100%" stopColor="#544943" stopOpacity={0.04} />
          </linearGradient>
          <linearGradient id="gRam" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#F0D2A3" stopOpacity={0.5} />
            <stop offset="100%" stopColor="#F0D2A3" stopOpacity={0.04} />
          </linearGradient>
          <linearGradient id="gGpu" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#EAD0A7" stopOpacity={0.5} />
            <stop offset="100%" stopColor="#EAD0A7" stopOpacity={0.04} />
          </linearGradient>
        </defs>
        <CartesianGrid stroke="#878782" strokeDasharray="3 3" vertical={false} />
        <XAxis dataKey="time" tick={{ fontSize: 10, fill: "#48464B" }} interval="preserveStartEnd" minTickGap={44} />
        <YAxis domain={[0, 100]} tick={{ fontSize: 10, fill: "#48464B" }} tickFormatter={(v) => `${v}%`} width={40} />
        <Tooltip
          contentStyle={{ background: "#CAC6BC", border: "1px solid #878782", borderRadius: 6, fontSize: 12 }}
          labelStyle={{ color: "#48464B" }}
          formatter={(v: number, n: string) => [`${v}%`, n]}
        />
        <Area type="monotone" dataKey="cpu" name="CPU" stroke="#544943" strokeWidth={2} fill="url(#gCpu)" isAnimationActive={false} />
        <Area type="monotone" dataKey="ram" name="Memory" stroke="#F0D2A3" strokeWidth={2} fill="url(#gRam)" isAnimationActive={false} />
        {hasGPU && (
          <Area type="monotone" dataKey="gpu" name="GPU memory" stroke="#EAD0A7" strokeWidth={2} fill="url(#gGpu)" isAnimationActive={false} />
        )}
      </AreaChart>
    </ResponsiveContainer>
  );
}

function Gauge({ used, total, unit }: { used: number; total: number; unit: string }) {
  const pct = total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0;
  const data = [{ name: unit, value: pct, fill: "#544943" }];
  return (
    <div className="relative">
      <ResponsiveContainer width="100%" height={150}>
        <RadialBarChart innerRadius="72%" outerRadius="100%" data={data} startAngle={220} endAngle={-40}>
          <PolarAngleAxis type="number" domain={[0, 100]} angleAxisId={0} tick={false} />
          <RadialBar background={{ fill: "#878782" }} dataKey="value" cornerRadius={8} angleAxisId={0} />
        </RadialBarChart>
      </ResponsiveContainer>
      <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
        <span className="text-2xl font-semibold text-colony-charcoal">{pct}%</span>
        <span className="font-mono text-xs text-colony-slate">
          {used.toFixed(1)} / {total} {unit}
        </span>
      </div>
    </div>
  );
}

function ScoreBar({ score, average }: { score: number; average: number }) {
  const max = Math.max(score, average, 1);
  return (
    <div className="space-y-3 pt-2">
      <LabeledBar label="You" value={score} max={max} color="bg-colony-core" />
      <LabeledBar label="Average" value={average} max={max} color="bg-colony-grayblue" />
    </div>
  );
}

function LabeledBar({ label, value, max, color }: { label: string; value: number; max: number; color: string }) {
  const pct = max > 0 ? Math.min(100, Math.round((value / max) * 100)) : 0;
  return (
    <div className="text-sm">
      <div className="mb-1 flex justify-between">
        <span className="text-colony-slate">{label}</span>
        <span className="font-mono text-xs text-colony-charcoal">{value.toFixed(0)}</span>
      </div>
      <div className="h-2 w-full rounded bg-colony-mist">
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
            <div className="text-sm font-medium text-colony-charcoal">
              {cfg.available ? "This machine is available to the Zone" : "This machine is paused"}
            </div>
            <p className="mt-1 max-w-md text-sm text-colony-slate">
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
            <div className="text-sm font-medium text-colony-charcoal">
              {cfg.only_when_idle ? `Contributing only after you have been away for ${cfg.idle_minutes} minutes` : "Contributing whenever this machine is available"}
            </div>
            <p className="mt-1 max-w-md text-sm text-colony-slate">
              When on, this machine contributes only while nobody is using its keyboard or mouse, and stops as soon as
              you come back. A Zone using it moves its work to its other machines.
            </p>
            {cfg.only_when_idle && (
              <label className="mt-2 flex items-center gap-2 text-sm text-colony-slate">
                Away for at least
                <select
                  value={cfg.idle_minutes}
                  onChange={(e) => applyNow({ ...cfg, idle_minutes: Number(e.target.value) })}
                  className="rounded border border-colony-mist bg-colony-cloud px-2 py-1 text-sm text-colony-charcoal outline-none focus:border-colony-core"
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
              <div className="text-sm font-medium text-colony-charcoal">
                {cfg.auto_start ? "Zonn Node starts in the background when you sign in" : "Zonn Node starts only when you open it"}
              </div>
              <p className="mt-1 max-w-md text-sm text-colony-slate">
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

      <Card title="Community sharing">
        <div className="flex items-center justify-between gap-4">
          <div>
            <div className="text-sm font-medium text-colony-charcoal">
              {cfg.share_community ? "Shared with the community pool" : "Only your own Zones use this machine"}
            </div>
            <p className="mt-1 max-w-md text-sm text-colony-slate">
              When on, Zonn may also include this machine in community Zones that serve other people. Their prompts and
              answers then pass through this machine while it computes its share. Turning this off takes the machine out of
              any community Zone at once.
            </p>
          </div>
          <Toggle on={cfg.share_community} busy={false} onChange={toggleShare} label="Share with the community pool" />
        </div>
      </Card>

      <Card title="Crash reports">
        <div className="flex items-center justify-between gap-4">
          <div>
            <div className="text-sm font-medium text-colony-charcoal">
              {cfg.crash_reports_enabled ? "Anonymized crash reports are on" : "Crash reports are off"}
            </div>
            <p className="mt-1 max-w-md text-sm text-colony-slate">
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
              className="w-full rounded border border-colony-mist bg-colony-cloud px-3 py-2 text-sm text-colony-charcoal outline-none focus:border-colony-core"
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
              className="w-full rounded border border-colony-mist bg-colony-cloud px-3 py-2 font-mono text-sm text-colony-charcoal outline-none focus:border-colony-core"
            />
          </Field>
          <div className="pt-1">
            <Row label="Node identity" value={state.enrolled ? cfg.node_id : "Not enrolled yet"} mono />
            <Row label="App version" value={state.app_version} mono />
          </div>
          <p className="text-xs text-colony-slate">
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
        <p className="mb-3 text-sm text-colony-slate">
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
            className="rounded bg-colony-core px-4 py-2 text-sm font-medium text-colony-cloud hover:bg-colony-deep disabled:opacity-60"
          >
            {saving ? "Saving..." : "Save settings"}
          </button>
          {saved && <span className="text-sm text-colony-slate">Saved and applied.</span>}
          {error && <span className="text-sm text-colony-slate">{error}</span>}
        </div>
      </Card>
    </div>
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
        on ? "bg-colony-core" : "bg-colony-mist"
      }`}
    >
      <span
        className={`inline-block h-5 w-5 transform rounded-full bg-colony-cloud transition-transform ${
          on ? "translate-x-6" : "translate-x-1"
        }`}
      />
    </button>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block text-sm">
      <span className="mb-1 block text-colony-slate">{label}</span>
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
        <button onClick={copy} className="rounded border border-colony-mist px-2 py-1 text-xs text-colony-slate hover:text-colony-charcoal">
          {copied ? "Copied" : "Copy log"}
        </button>
      }
    >
      <div className="max-h-[28rem] overflow-y-auto font-mono text-xs">
        {state.events.length === 0 && <div className="text-colony-slate">No activity yet.</div>}
        {state.events.map((e, i) => (
          <div key={i} className="flex gap-3 border-b border-colony-mist py-1.5 last:border-0">
            <span className="shrink-0 text-colony-slate">{new Date(e.time).toLocaleTimeString()}</span>
            <LevelTag level={e.level} />
            <span className="text-colony-charcoal">{e.message}</span>
          </div>
        ))}
      </div>
    </Card>
  );
}

function LevelTag({ level }: { level: string }) {
  const map: Record<string, string> = {
    INFO: "bg-colony-softblue text-colony-deep",
    WARN: "bg-colony-warmbeige text-colony-navy",
    ERROR: "bg-colony-indigo text-colony-cloud",
  };
  const cls = map[level] ?? "bg-colony-mist text-colony-navy";
  return <span className={`shrink-0 rounded px-1.5 ${cls}`}>{level}</span>;
}

function Card({ title, action, children }: { title: string; action?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="rounded-md border border-colony-mist bg-colony-nearwhite p-5">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-colony-slate">{title}</h2>
        {action}
      </div>
      <div className="space-y-2">{children}</div>
    </section>
  );
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-4 text-sm">
      <span className="text-colony-slate">{label}</span>
      <span className={`text-right text-colony-charcoal ${mono ? "font-mono text-xs" : ""}`}>{value}</span>
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
        <span className="text-colony-slate">
          {label}
          {note && <span className="ml-2 text-xs text-colony-slate">({note})</span>}
        </span>
        <span className="font-mono text-xs text-colony-charcoal">
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
    "w-full rounded border border-colony-mist bg-colony-cloud px-3 py-2 font-mono text-sm text-colony-charcoal outline-none focus:border-colony-core";
  return (
    <div className="mb-6">
      <Card title="Enroll this machine">
        <p className="text-sm text-colony-slate">
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
          {error && <p className="text-sm text-colony-indigo">{error}</p>}
          <button
            type="submit"
            disabled={busy || !code.trim()}
            className="rounded bg-colony-core px-4 py-2 text-sm font-medium text-colony-cloud hover:bg-colony-deep disabled:opacity-60"
          >
            {busy ? "Enrolling..." : "Enroll this machine"}
          </button>
        </form>
      </Card>
    </div>
  );
}

function Banner({ text }: { text: string }) {
  return (
    <div className="rounded-md border border-colony-mist bg-colony-paleblue px-4 py-3 text-sm text-colony-midnight">
      {text}
    </div>
  );
}
