import { apiRequest } from "../../shared/api/server-api";
import type { ManagedUser, UserMutation } from "../../shared/types/managed-user";

export const listUsers = (path: string, token: string) => apiRequest<ManagedUser[]>(path, {}, token);

export const createUser = (values: UserMutation, token: string) =>
  apiRequest<ManagedUser>("/api/admin/users", { method: "POST", body: JSON.stringify(values) }, token);

export const updateUser = (userID: string, values: UserMutation, token: string) =>
  apiRequest<ManagedUser>(`/api/admin/users/${encodeURIComponent(userID)}`, { method: "PUT", body: JSON.stringify(values) }, token);

export const deleteUser = (userID: string, token: string) =>
  apiRequest<{ deleted: boolean }>(`/api/admin/users/${encodeURIComponent(userID)}`, { method: "DELETE" }, token);
