import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../app/auth";

export default function Layout() {
  const { logout } = useAuth();
  const nav = useNavigate();

  const linkClass = ({ isActive }: { isActive: boolean }) =>
    `rounded-lg px-3 py-1.5 text-sm font-medium transition ${
      isActive ? "bg-indigo-50 text-indigo-700" : "text-slate-600 hover:bg-slate-100"
    }`;

  return (
    <div className="min-h-screen">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex max-w-4xl items-center justify-between px-6 py-3">
          <NavLink to="/" className="flex items-center gap-2 text-lg font-semibold text-slate-900">
            <span className="grid h-7 w-7 place-items-center rounded-lg bg-indigo-600 text-sm text-white">L</span>
            LedgerX
          </NavLink>
          <nav className="flex items-center gap-1">
            <NavLink to="/" end className={linkClass}>Accounts</NavLink>
            <NavLink to="/exports" className={linkClass}>Exports</NavLink>
            <button
              className="btn-ghost ml-2 rounded-lg px-3 py-1.5 text-sm font-medium"
              onClick={() => { logout(); nav("/login"); }}
            >
              Log out
            </button>
          </nav>
        </div>
      </header>
      <main className="mx-auto max-w-4xl px-6 py-8">
        <Outlet />
      </main>
    </div>
  );
}
