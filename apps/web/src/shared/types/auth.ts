export type User = {
  id: string;
  username: string;
  display_name: string;
  role: "admin" | "user";
};

export type Session = {
  token: string;
  user: User;
};
