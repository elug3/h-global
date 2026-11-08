import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { Navigate, useLocation } from "react-router";
import { api, call, type Staff } from "./api/client";
import { t } from "./i18n/ko";

type Auth = {
  staff: Staff | null;
  ready: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  can: (role: "cs" | "manager" | "owner") => boolean;
};

const rank: Record<string, number> = { cs: 1, manager: 2, owner: 3 };
const AuthContext = createContext<Auth | null>(null);

/** Holds the signed-in staff member. The session itself is an HttpOnly cookie. */
export function AuthProvider({ children }: { children: ReactNode }) {
  const [staff, setStaff] = useState<Staff | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    call(api.GET("/studio-api/auth/me"))
      .then(setStaff)
      .catch(() => setStaff(null))
      .finally(() => setReady(true));
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    setStaff(await call(api.POST("/studio-api/auth/login", { body: { email, password } })));
  }, []);

  const logout = useCallback(async () => {
    await api.POST("/studio-api/auth/logout").catch(() => undefined);
    setStaff(null);
  }, []);

  const can = useCallback((role: "cs" | "manager" | "owner") => !!staff && rank[staff.role] >= rank[role], [staff]);

  return <AuthContext.Provider value={{ staff, ready, login, logout, can }}>{children}</AuthContext.Provider>;
}

export function useAuth(): Auth {
  const a = useContext(AuthContext);
  if (!a) throw new Error("useAuth outside AuthProvider");
  return a;
}

/** Sends signed-out visitors to the login page. */
export function RequireAuth({ children }: { children: ReactNode }) {
  const { staff, ready } = useAuth();
  const location = useLocation();
  if (!ready) return <div className="page-center muted">{t.app.loading}</div>;
  if (!staff) return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  return <>{children}</>;
}
