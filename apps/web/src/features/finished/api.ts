import { apiRequest } from "../../shared/api/server-api";

export type FinishedWorkDownloadBatch = {
  download_url: string;
  file_name: string;
  file_count: number;
  expires_at: string;
};

export function createFinishedWorkDownload(workIDs: string[], token: string) {
  return apiRequest<FinishedWorkDownloadBatch>("/api/workbench/works/download-batches", {
    method: "POST",
    body: JSON.stringify({ work_ids: workIDs })
  }, token);
}
