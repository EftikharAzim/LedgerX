import { Link, NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../app/auth-context";
import Icon, { BrandMark } from "./Icon";

export default function Layout() {
  const { logout } = useAuth();
  const nav = useNavigate();

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
        <div className="workspace-label">
          <span className="workspace-avatar">P</span>
          <div>
            <strong>Personal workspace</strong>
          </div>
        </div>
        <div className="nav-caption">WORKSPACE</div>
        <nav className="primary-nav" aria-label="Main navigation">
          <NavLink to="/" end>
            <Icon name="overview" />
            <span>Overview</span>
          </NavLink>
          <NavLink to="/activity">
            <Icon name="list" />
            <span>Activity</span>
          </NavLink>
          <NavLink to="/exports">
            <Icon name="export" />
            <span>Reports</span>
          </NavLink>
        </nav>
        <div className="sidebar-bottom">
          <div className="workspace-management">
            <Link to="/workspace">
              <Icon name="shield" width="16" height="16" />
              Backup & restore
            </Link>
            <Link to="/imports">
              <Icon name="download" width="16" height="16" />
              Import CSV
            </Link>
            <Link to="/accounts">
              <Icon name="wallet" width="16" height="16" />
              Manage accounts
            </Link>
            <Link to="/categories">
              <Icon name="tag" width="16" height="16" />
              Manage categories
            </Link>
          </div>
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
          <Outlet />
        </main>
        <footer className="workspace-footer">
          <Link to="/welcome" className="getting-started-link">
            Getting started
            <Icon name="arrow" width="13" height="13" />
          </Link>
        </footer>
      </div>
    </div>
  );
}
