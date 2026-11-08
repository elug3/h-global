import { NavLink, Outlet } from "react-router";
import { api, call } from "../api/client";
import { useAuth } from "../auth";
import { t } from "../i18n/ko";
import { ORDER_EVENTS, useDebounced, useEventStream, useLoad } from "../lib/hooks";
import { Icon, type IconName } from "./Icon";
import { TODO_STATUSES } from "./order";

type Item = { to: string; label: string; icon: IconName; owner?: boolean; badge?: "orders" };
const items: Item[] = [
  { to: "/shows", label: t.nav.shows, icon: "radio" },
  { to: "/orders", label: t.nav.orders, icon: "receipt", badge: "orders" },
  { to: "/products", label: t.nav.catalog, icon: "box" },
  { to: "/staff", label: t.nav.staff, icon: "users", owner: true },
];

/** Left rail plus the page. Desktop only (H-Global Studio is used on a PC). */
export function Layout() {
  const { staff, logout, can } = useAuth();
  // Orders waiting for staff (paid, inspecting, customs check), kept current
  // by the order event stream.
  const counts = useLoad(() => call(api.GET("/studio-api/orders/counts")), []);
  const recount = useDebounced(() => void counts.reload(), 500);
  useEventStream("/studio-api/orders/events", "now", ORDER_EVENTS, recount, recount);
  const c = counts.data?.counts as Record<string, number> | undefined;
  const todo = c ? TODO_STATUSES.reduce((n, s) => n + (c[s] ?? 0), 0) : 0;

  return (
    <div className="shell">
      <nav className="rail" aria-label="메뉴">
        <div className="rail-logo">HG</div>
        {items
          .filter((it) => !it.owner || can("owner"))
          .map((it) => (
            <NavLink key={it.to} to={it.to} className={({ isActive }) => "rail-item" + (isActive ? " on" : "")}>
              <Icon name={it.icon} />
              <span>{it.label}</span>
              {it.badge === "orders" && todo > 0 && (
                <em className="rail-badge" aria-label={`${t.orders.tabTodo} ${todo}`}>
                  {todo > 99 ? "99+" : todo}
                </em>
              )}
            </NavLink>
          ))}
        <div className="rail-spacer" />
        {staff && (
          <div className="rail-user" title={staff.email}>
            <span className="rail-name">{staff.name}</span>
            <span className="rail-role">{t.roles[staff.role] ?? staff.role}</span>
          </div>
        )}
        <button className="rail-item rail-signout" onClick={() => void logout()}>
          <Icon name="logout" />
          <span>{t.nav.signOut}</span>
        </button>
      </nav>
      <main className="main">
        <Outlet />
      </main>
    </div>
  );
}
