import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../app/auth-context";
import Icon, { BrandMark } from "./Icon";

export default function Layout() {
  const { logout } = useAuth();
  const nav = useNavigate();
  const { pathname } = useLocation();
  const section =
    pathname === "/"
      ? "Overview"
      : pathname === "/exports"
        ? "Exports"
        : "Accounts";
  const date = new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(new Date());

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
            <span>A little more clarity.</span>
          </div>
        </div>
        <div className="nav-caption">WORKSPACE</div>
        <nav className="primary-nav" aria-label="Main navigation">
          <NavLink to="/" end>
            <Icon name="overview" />
            <span>Overview</span>
          </NavLink>
          <NavLink to="/accounts">
            <Icon name="wallet" />
            <span>Accounts</span>
          </NavLink>
          <NavLink to="/exports">
            <Icon name="export" />
            <span>Exports</span>
          </NavLink>
        </nav>
        <div className="sidebar-bottom">
          <div className="sidebar-note">
            <Icon
              name="sparkle"
              className="note-spark"
              width="31"
              height="31"
            />
            <p>
              Good records.
              <br />
              <em>Clearer possibilities.</em>
            </p>
            <span className="sidebar-note-line" />
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
          <span className="sidebar-footnote">YOUR MONEY. IN BALANCE.</span>
        </div>
      </aside>
      <div className="workspace-main">
        <header className="workspace-header">
          <div className="breadcrumb">
            <span>Workspace</span>
            <Icon name="chevron" width="13" height="13" />
            <strong>{section}</strong>
          </div>
          <div className="header-date">
            <Icon name="calendar" width="16" height="16" />
            <time>{date}</time>
          </div>
        </header>
        <main id="main-content" className="workspace-content" tabIndex={-1}>
          <Outlet />
        </main>
        <footer className="workspace-footer">
          <span>
            <span className="footer-mark" aria-hidden="true">
              ＝
            </span>{" "}
            A little clarity goes a long way.
          </span>
          <span>Made to keep things balanced.</span>
        </footer>
      </div>
    </div>
  );
}
