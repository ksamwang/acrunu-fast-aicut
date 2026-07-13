import { apiRequest } from "../../shared/api/server-api";
import type { Task } from "../../shared/types/task";

export function listTasks(path: string, token: string) {
  return apiRequest<Task[]>(path, {}, token);
}

export function createTestTask(token: string) {
  return apiRequest<Task>("/api/tasks/test", { method: "POST" }, token);
}

export function getTask(taskID: string, token: string) {
  return apiRequest<Task>(`/api/tasks/${taskID}`, {}, token);
}
