export interface Specs {
  os: string;
  arch: string;
  cpu_cores: number;
  ram_gb: number;
  disk_gb: number;
  gpu_model: string;
  gpu_memory_gb: number;
}

export interface Allocation {
  cpu_cores: number;
  ram_gb: number;
  gpu_memory_gb: number;
  bandwidth_mbps: number;
}

export interface Utilization {
  cpu_used: number;
  ram_used_gb: number;
  gpu_mem_used_gb: number;
  gpu_temp_c: number;
}

export interface ColonyUsage {
  cpu_cores: number;
  ram_gb: number;
  active: boolean;
}

export interface ActivityEvent {
  time: string;
  level: string;
  message: string;
}

export interface Ranking {
  rank: number;
  active_nodes: number;
  contribution_score: number;
  average_score: number;
}

export interface State {
  node_name: string;
  node_id: string;
  connection: "CONNECTED" | "CONNECTING" | "DISCONNECTED" | "PAUSED" | "UNENROLLED" | "UPDATE_REQUIRED";
  status: "ONLINE" | "OFFLINE";
  available: boolean;
  colony_id: string;
  coordinator_url: string;
  specs: Specs;
  allocation: Allocation;
  utilization: Utilization;
  colony_usage: ColonyUsage;
  ranking: Ranking;
  events: ActivityEvent[];
  // enrolled is true once this machine holds a credential for the configured
  // Coordinator; until then it waits for an enrollment code.
  enrolled: boolean;
  // notice explains something the contributor has to act on, such as an update.
  notice: string;
  app_version: string;
  // waiting_for_idle is true while Only when idle holds the node back.
  waiting_for_idle: boolean;
  idle_note: string;
  // desktop is true when the desktop app runs the node (start at sign in).
  desktop: boolean;
  update: UpdateState;
}

// UpdateState is a newer Zonn Node published on the backend.
export interface UpdateState {
  available: boolean;
  required: boolean;
  version: string;
  size: number;
  status: "" | "downloading" | "opened" | "error";
  done: number;
  error?: string;
}

export interface Config {
  node_id: string;
  node_name: string;
  coordinator_url: string;
  allocation: Allocation;
  available: boolean;
  configured: boolean;
  only_when_idle: boolean;
  idle_minutes: number;
  auto_start: boolean;
  crash_reports_enabled: boolean;
  share_community: boolean;
}
