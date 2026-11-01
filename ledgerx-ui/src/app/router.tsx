import { createBrowserRouter, RouterProvider } from "react-router-dom";
import { lazy, Suspense } from "react";
const Register = lazy(() => import("../pages/Register"));
const Accounts = lazy(() => import("../pages/Accounts"));
const Login = lazy(() => import("../pages/Login"));
import ProtectedRoute from "../components/ProtectedRoute";
import Layout from "../components/Layout";
const Welcome = lazy(() => import("../pages/Welcome"));
const Categories = lazy(() => import("../pages/Categories"));
const Overview = lazy(() => import("../pages/Overview"));
const Activity = lazy(() => import("../pages/Activity"));
import NewWorkspace from "../pages/NewWorkspace";
const Recover = lazy(() => import("../pages/Recover"));

const AccountDetails = lazy(() => import("../pages/AccountDetails"));
const Reports = lazy(() => import("../pages/Reports"));
const Budgets = lazy(() => import("../pages/Budgets"));
const Exports = lazy(() => import("../pages/Exports"));
const Schedules = lazy(() => import("../pages/Schedules"));
const Planned = lazy(() => import("../pages/Planned"));
const Imports = lazy(() => import("../pages/Imports"));
const Workspace = lazy(() => import("../pages/Workspace"));
const Security = lazy(() => import("../pages/Security"));

const router = createBrowserRouter([
  { path: "/login", element: <Login /> },
  { path: "/register", element: <Register /> },
  { path: "/new-workspace", element: <NewWorkspace /> },
  { path: "/recover", element: <Recover /> },
  {
    element: (
      <ProtectedRoute>
        <Layout />
      </ProtectedRoute>
    ),
    children: [
      { path: "/", element: <Overview /> },
      { path: "/activity", element: <Activity /> },
      { path: "/schedules", element: <Schedules /> },
      { path: "/planned", element: <Planned /> },
      { path: "/imports", element: <Imports /> },
      { path: "/workspace", element: <Workspace /> },
      { path: "/security", element: <Security /> },
      { path: "/accounts", element: <Accounts /> },
      { path: "/accounts/:id", element: <AccountDetails /> },
      { path: "/reports", element: <Reports /> },
      { path: "/budgets", element: <Budgets /> },
      { path: "/exports", element: <Exports /> },
      { path: "/categories", element: <Categories /> },
      { path: "/welcome", element: <Welcome /> },
    ],
  },
]);

export default function AppRouter() {
  return (
    <Suspense
      fallback={
        <main className="recovery-page" role="status">
          Loading LedgerX…
        </main>
      }
    >
      <RouterProvider router={router} />
    </Suspense>
  );
}
