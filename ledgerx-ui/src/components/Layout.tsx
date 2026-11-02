import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { Suspense } from "react";
import { useAuth } from "../app/auth-context";
import Icon, { BrandMark } from "./Icon";

export default function Layout() {
  const { logout } = useAuth();
  const nav = useNavigate();
  const { pathname } = useLocation();
  const moreActive = !["/", "/activity", "/budgets", "/reports"].includes(
    pathname,
  );

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <aside className="sidebar">
        <NavLink to="/" className="brand" aria-label="LedgerX home">
          <BrandMark />
          <span>
            Ledger<span className="brand-x">X</span>
          </span>
        </NavLink>
        <nav className="primary-nav" aria-label="Main navigation">
          <NavLink to="/" end>
            <Icon name="overview" />
            <span>Overview</span>
          </NavLink>
          <NavLink to="/activity">
            <Icon name="list" />
            <span>Activity</span>
          </NavLink>
          <NavLink to="/budgets">
            <Icon name="wallet" />
            <span>Budgets</span>
          </NavLink>
          <NavLink to="/reports">
            <Icon name="export" />
            <span>Reports</span>
          </NavLink>
          <NavLink
            to="/more"
            className={({ isActive }) =>
              isActive || moreActive ? "active" : undefined
            }
          >
            <Icon name="more" />
            <span>More</span>
          </NavLink>
        </nav>
        <div className="sidebar-bottom">
          <button
            className="logout-button"
            onClick={() => {
              logout();
              nav("/login");
            }}
          >
            <Icon name="logout" />
            <span>Log out</span>
          </button>
        </div>
      </aside>
      <div className="workspace-main">
        <main id="main-content" className="workspace-content" tabIndex={-1}>
          <Suspense fallback={<p role="status">Loading page…</p>}>
            <Outlet />
          </Suspense>
        </main>
      </div>
    </div>
  );
}
