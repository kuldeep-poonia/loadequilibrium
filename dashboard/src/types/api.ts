import type { ControlPlaneState } from "./tick";

export interface HealthResponse {
  status: string;
  component: string;
  clients: number;
}

export interface ControlToggleRequest {
  enabled?: boolean;
  service_id?: string;
}

export interface ControlToggleResponse {
  status: string;
  action: string;
  actuation_enabled: boolean;
  actuator_configured: boolean;
  control_plane: ControlPlaneState;
}

export interface ChaosRunRequest {
  service_id?: string;
  duration_ticks?: number;
  request_factor?: number;
  latency_factor?: number;
}

export interface ChaosRunResponse {
  status: string;
  action: string;
  target_service: string;
  start_tick: number;
  until_tick: number;
  request_factor: number;
  latency_factor: number;
  scenario_mode: string;
  control_plane: ControlPlaneState;
}

export interface ReplayBurstRequest {
  service_id?: string;
  duration_ticks?: number;
  factor?: number;
}

export interface ReplayBurstResponse {
  status: string;
  action: string;
  target_service: string;
  start_tick: number;
  until_tick: number;
  factor: number;
  scenario_mode: string;
  control_plane: ControlPlaneState;
}

export interface PolicyUpdateRequest {
  preset: string;
}

export interface PolicyUpdateResponse {
  status: string;
  domain: string;
  preset: string;
  control_plane: ControlPlaneState;
}

export interface RuntimeStepResponse {
  status: string;
  domain: string;
  tick: number;
  control_plane: ControlPlaneState;
}

export interface SandboxTriggerRequest {
  type?: string;
  duration_ticks?: number;
}

export interface SandboxTriggerResponse {
  status: string;
  domain: string;
  type: string;
  until_tick: number;
  duration_ticks: number;
  control_plane: ControlPlaneState;
}

export interface SimulationControlRequest {
  action: "run" | "start" | "force" | "stop" | "reset";
  duration_ticks?: number;
}

export interface SimulationControlResponse {
  status: string;
  domain: string;
  action: string;
  until_tick?: number;
  duration_ticks?: number;
  control_plane: ControlPlaneState;
}

export interface IntelligenceRolloutRequest {
  duration_ticks?: number;
}

export interface IntelligenceRolloutResponse {
  status: string;
  domain: string;
  until_tick: number;
  duration_ticks: number;
  control_plane: ControlPlaneState;
}

export interface AlertAckRequest {
  alert_id: string;
}

export interface AlertAckResponse {
  status: string;
  domain: string;
  alert_id: string;
  acknowledged_alert_count: number;
  control_plane: ControlPlaneState;
}

export interface ApiError {
  error: string;
}

export type PolicyPreset = "balanced" | "aggressive" | "conservative" | "safe" | "performance";
