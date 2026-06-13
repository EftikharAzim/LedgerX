import { createBrowserRouter, RouterProvider } from "react-router-dom";
import Register from "../pages/Register";
import Accounts from "../pages/Accounts";
import Login from "../pages/Login";
import AccountDetails from "../pages/AccountDetails";
import Exports from "../pages/Exports";
import ProtectedRoute from "../components/ProtectedRoute";
import Layout from "../components/Layout";

const router = createBrowserRouter([
  { path: "/login", element: <Login /> },
  { path: "/register", element: <Register /> },
  {
    element: (
      <ProtectedRoute>
        <Layout />
      </ProtectedRoute>
    ),
    children: [
      { path: "/", element: <Accounts /> },
      { path: "/accounts/:id", element: <AccountDetails /> },
      { path: "/exports", element: <Exports /> },
    ],
  },
]);

export default function AppRouter() {
  return <RouterProvider router={router} />;
}
