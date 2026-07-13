export type Task = {
  id: string;
  product_id?: string;
  created_by_user_id?: string;
  task_type: string;
  status: string;
  payload_summary?: Record<string, unknown>;
  asset_id?: string;
  duration_ms?: number;
  error_message?: string;
  retry_count: number;
  created_at: string;
  updated_at?: string;
  started_at?: string;
  finished_at?: string;
};
