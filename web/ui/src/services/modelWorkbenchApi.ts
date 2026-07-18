import { api } from '@/services/api';

export type ModelWorkbenchItem = Record<string, unknown>;

export type ModelVersionRecord = {
  model_version: string;
  model_id: string;
  status: string;
  feature_set_id: string;
  artifact_uri: string;
  metrics?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type ModelActionRecord = {
  job_id: string;
  action_id: string;
  action: string;
  target: string;
  version?: string;
  status: string;
  requested_by: string;
  created_at: string;
};

export type ModelWorkbench = {
  model: Record<string, unknown>;
  versions: ModelVersionRecord[];
  items: Record<string, ModelWorkbenchItem[]>;
  actions: ModelActionRecord[];
  source: 'postgresql' | string;
};

export async function fetchModelWorkbench(modelId: string): Promise<ModelWorkbench> {
  const response = await api.get<{ success: boolean; data: ModelWorkbench }>(`/v1/models/${encodeURIComponent(modelId)}/workbench`);
  return response.data.data;
}
