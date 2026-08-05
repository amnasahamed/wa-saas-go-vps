"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { api, ApiError, type User, type Workspace } from "@/lib/api";

type AuthStatus = "loading" | "authenticated" | "anonymous";

type AuthContextValue = {
  status: AuthStatus;
  user: User | null;
  workspace: Workspace | null;
  refresh: () => Promise<void>;
  login: (email: string, password: string) => Promise<{ ok: true } | { ok: false; code: string }>;
  signup: (input: {
    email: string;
    password: string;
    workspaceName: string;
  }) => Promise<{ ok: true; verifyToken?: string } | { ok: false; code: string; message: string }>;
  logout: () => Promise<void>;
  verifyEmail: (token: string) => Promise<{ ok: true } | { ok: false; code: string }>;
};

const AuthContext = React.createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [status, setStatus] = React.useState<AuthStatus>("loading");
  const [user, setUser] = React.useState<User | null>(null);
  const [workspace, setWorkspace] = React.useState<Workspace | null>(null);

  const refresh = React.useCallback(async () => {
    try {
      const me = await api.get<{ user: User; workspace: Workspace }>("/auth/me");
      setUser(me.user);
      setWorkspace(me.workspace);
      setStatus("authenticated");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setUser(null);
        setWorkspace(null);
        setStatus("anonymous");
      } else {
        // Network error: treat as anonymous but don't crash.
        setUser(null);
        setWorkspace(null);
        setStatus("anonymous");
      }
    }
  }, []);

  React.useEffect(() => {
    void refresh();
  }, [refresh]);

  const login: AuthContextValue["login"] = React.useCallback(async (email, password) => {
    try {
      await api.post("/auth/login", { email, password });
      await refresh();
      return { ok: true };
    } catch (err) {
      if (err instanceof ApiError) {
        const code = (err.body as { error?: { code?: string } } | null)?.error?.code;
        return { ok: false, code: code ?? "unknown" };
      }
      return { ok: false, code: "network" };
    }
  }, [refresh]);

  const signup: AuthContextValue["signup"] = React.useCallback(
    async ({ email, password, workspaceName }) => {
      try {
        const res = await api.post<{ verify_token?: string }>("/auth/signup", {
          email,
          password,
          workspace_name: workspaceName,
        });
        return { ok: true, verifyToken: res.verify_token };
      } catch (err) {
        if (err instanceof ApiError) {
          const body = (err.body ?? {}) as { error?: { code?: string; message?: string } };
          return {
            ok: false,
            code: body.error?.code ?? "unknown",
            message: body.error?.message ?? err.message,
          };
        }
        return { ok: false, code: "network", message: "Network error" };
      }
    },
    []
  );

  const logout = React.useCallback(async () => {
    try {
      await api.post("/auth/logout");
    } catch {
      // ignore — best-effort
    }
    setUser(null);
    setWorkspace(null);
    setStatus("anonymous");
    router.push("/login");
  }, [router]);

  const verifyEmail: AuthContextValue["verifyEmail"] = React.useCallback(async (token) => {
    try {
      await api.post("/auth/verify-email", { token });
      return { ok: true };
    } catch (err) {
      if (err instanceof ApiError) {
        const code = (err.body as { error?: { code?: string } } | null)?.error?.code;
        return { ok: false, code: code ?? "unknown" };
      }
      return { ok: false, code: "network" };
    }
  }, []);

  const value = React.useMemo<AuthContextValue>(
    () => ({
      status,
      user,
      workspace,
      refresh,
      login,
      signup,
      logout,
      verifyEmail,
    }),
    [status, user, workspace, refresh, login, signup, logout, verifyEmail]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = React.useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used inside AuthProvider");
  }
  return ctx;
}
