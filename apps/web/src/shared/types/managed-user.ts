import type { User } from "./auth";

export type ManagedUser = User & {
  email?: string;
  status: "active" | "disabled";
  last_login_at?: string;
  created_at: string;
  updated_at: string;
};

export type UserMutation = {
  username: string;
  display_name: string;
  email?: string;
  password?: string;
  role: User["role"];
};
