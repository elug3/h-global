import { Link, Outlet, useLoaderData, useLocation } from "react-router";
import { serverApi } from "../api/client";
import { Icon, LivePill } from "../components/ui";
import { t } from "../i18n/ko";
import type { Route } from "./+types/layout";

/** Who is signed in and whether a stream is live, for the header. */
export async function loader({ request }: Route.LoaderArgs) {
  const api = serverApi(request);
  const [me, current] = await Promise.all([
    api.GET("/api/me").then((r) => (r.response.ok ? (r.data ?? null) : null)).catch(() => null),
    api.GET("/api/shows/current").then((r) => r.data?.show ?? null).catch(() => null),
  ]);
  return { me, live: current?.status === "live" ? { id: current.id } : null };
}

export default function SiteLayout() {
  const { me, live } = useLoaderData<typeof loader>();
  const here = useLocation().pathname;
  const onLivePage = live && here === `/shows/${live.id}`;
  return (
    <>
      <header className="mh">
        <Link to="/" className="logo" aria-label={t.app.name}>
          {t.app.name}
        </Link>
        <nav className="mh-r" aria-label="메뉴">
          {live && !onLivePage && (
            <Link to={`/shows/${live.id}`} aria-label="LIVE 방송 보기">
              <LivePill />
            </Link>
          )}
          <Link to={me ? "/orders" : `/login?next=${encodeURIComponent(here)}`} className="icon-link" aria-label={me ? t.nav.myOrders : t.nav.login}>
            <Icon name={me ? "receipt" : "user"} />
          </Link>
        </nav>
      </header>
      <Outlet />
    </>
  );
}
