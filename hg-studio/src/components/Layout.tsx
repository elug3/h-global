import { NavLink, Outlet } from "react-router";
import { useAuth } from "../auth";
import { t } from "../i18n/ko";
import { Icon, type IconName } from "./Icon";

const items: { to?: string; label: string; icon: IconName }[] = [
  { to: "/shows", label: t.nav.shows, icon: "radio" },
  { label: t.nav.catalog, icon: "box" },
  { label: t.nav.orders, icon: "receipt" },
  { label: t.nav.staff, icon: "users" },
];

/** Left rail plus the page. Desktop only (H-Global Studio is used on a PC). */
export function Layout() {
  const { staff, logout } = useAuth();
  return (
    <div className="shell">
      <nav className="rail" aria-label="메뉴">
        <div className="rail-logo">HG</div>
        {items.map((it) =>
          it.to ? (
            <NavLink key={it.label} to={it.to} className={({ isActive }) => "rail-item" + (isActive ? " on" : "")}>
              <Icon name={it.icon} />
              <span>{it.label}</span>
            </NavLink>
          ) : (
            <span key={it.label} className="rail-item disabled" title={t.app.comingSoon}>
              <Icon name={it.icon} />
              <span>{it.label}</span>
            </span>
          ),
        )}
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
